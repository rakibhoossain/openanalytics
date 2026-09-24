package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin WebSocket connections (e.g. Vite dev on :3000 to :8081)
	},
}

// Client represents an active WebSocket connection.
type Client struct {
	hub      *WebSocketHub
	conn     *websocket.Conn
	send     chan []byte
	tenantID string
	shopID   string
	topic    string // "visitors", "events", "notifications", "organization"
}

// WebSocketHub manages active client connections and event routing.
type WebSocketHub struct {
	mu            sync.RWMutex
	rdb           *redis.Client
	queryService  *Service
	subscriptions map[string]map[*Client]bool // key format: "{topic}:{shopID}"
	register      chan *Client
	unregister    chan *Client
	broadcast     chan wsBroadcastMessage
}

type wsBroadcastMessage struct {
	key     string // "{topic}:{shopID}"
	payload []byte
}

// NewWebSocketHub creates a new WebSocket management hub.
func NewWebSocketHub(rdb *redis.Client, qs *Service) *WebSocketHub {
	return &WebSocketHub{
		rdb:           rdb,
		queryService:  qs,
		subscriptions: make(map[string]map[*Client]bool),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		broadcast:     make(chan wsBroadcastMessage, 256),
	}
}

// Start runs the hub message routing and Redis subscription loops.
func (h *WebSocketHub) Start(ctx context.Context) {
	// 1. Hub connection management loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case client := <-h.register:
				h.mu.Lock()
				key := fmt.Sprintf("%s:%s", client.topic, client.shopID)
				if _, ok := h.subscriptions[key]; !ok {
					h.subscriptions[key] = make(map[*Client]bool)
				}
				h.subscriptions[key][client] = true
				h.mu.Unlock()

			case client := <-h.unregister:
				h.mu.Lock()
				key := fmt.Sprintf("%s:%s", client.topic, client.shopID)
				if clients, ok := h.subscriptions[key]; ok {
					if _, exists := clients[client]; exists {
						delete(clients, client)
						close(client.send)
						if len(clients) == 0 {
							delete(h.subscriptions, key)
						}
					}
				}
				h.mu.Unlock()

			case msg := <-h.broadcast:
				h.mu.RLock()
				if clients, ok := h.subscriptions[msg.key]; ok {
					for client := range clients {
						select {
						case client.send <- msg.payload:
						default:
							close(client.send)
							delete(clients, client)
						}
					}
				}
				h.mu.RUnlock()
			}
		}
	}()

	// 2. Redis Pub/Sub listener for distributed events across workers
	if h.rdb != nil {
		go func() {
			pubsub := h.rdb.PSubscribe(ctx, "analytics:live:*")
			defer pubsub.Close()

			ch := pubsub.Channel()
			log.Println("[WebSocket Hub] Subscribed to Redis Pub/Sub topic pattern analytics:live:*")

			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					// Channels format: "analytics:live:{topic}:{shopID}"
					parts := strings.Split(msg.Channel, ":")
					if len(parts) >= 4 {
						topic := parts[2]
						shopID := parts[3]
						h.broadcast <- wsBroadcastMessage{
							key:     fmt.Sprintf("%s:%s", topic, shopID),
							payload: []byte(msg.Payload),
						}
					}
				}
			}
		}()
	}
}

// BroadcastVisitors directly dispatches an updated active visitor count to subscribed clients.
func (h *WebSocketHub) BroadcastVisitors(shopID string, count int64) {
	// Both superjson and raw string support: openpanel start parses String(count) or superjson
	payload := fmt.Appendf(nil, `{"json":%d}`, count)
	h.broadcast <- wsBroadcastMessage{
		key:     fmt.Sprintf("visitors:%s", shopID),
		payload: payload,
	}
}

// BroadcastEvents directly dispatches a new event notification badge to subscribed clients.
func (h *WebSocketHub) BroadcastEvents(shopID string, count int) {
	// SuperJSON format expected by openpanel useWS: {"json":{"count":N}}
	envelope := map[string]any{
		"json": map[string]any{
			"count": count,
		},
	}
	bytes, _ := json.Marshal(envelope)
	h.broadcast <- wsBroadcastMessage{
		key:     fmt.Sprintf("events:%s", shopID),
		payload: bytes,
	}
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			// Add queued messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				_, _ = w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// extractTargetIDs extracts shopID and tenantID from path or query params.
func extractTargetIDs(r *http.Request) (string, string) {
	// 1. Path param from Chi: {shopId}, {projectId}, {organizationId}, {tenantId}
	shopID := chi.URLParam(r, "shopId")
	if shopID == "" {
		shopID = chi.URLParam(r, "projectId")
	}
	tenantID := chi.URLParam(r, "tenantId")
	if tenantID == "" {
		tenantID = chi.URLParam(r, "organizationId")
	}

	// 2. Query param fallbacks
	if shopID == "" {
		shopID = r.URL.Query().Get("shopId")
		if shopID == "" {
			shopID = r.URL.Query().Get("projectId")
		}
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = r.URL.Query().Get("organizationId")
		}
	}

	// 3. Fallback defaults if unspecified
	if shopID == "" {
		shopID = "018e69d0-7a89-7000-8b1a-200000000002"
	}
	if tenantID == "" {
		tenantID = "018e69d0-7a89-7000-8b1a-200000000001"
	}

	return shopID, tenantID
}

// ServeLiveVisitors handles GET /live/visitors/{shopId} WebSocket upgrade.
func (h *WebSocketHub) ServeLiveVisitors(w http.ResponseWriter, r *http.Request) {
	shopID, tenantID := extractTargetIDs(r)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade visitors failed: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		tenantID: tenantID,
		shopID:   shopID,
		topic:    "visitors",
	}
	// Immediately buffer current live visitor count upon connection
	var count int64 = 0
	if h.queryService != nil {
		if sUUID, err := uuid.Parse(shopID); err == nil {
			if tUUID, err := uuid.Parse(tenantID); err == nil {
				if live, err := h.queryService.GetLiveVisitors(r.Context(), tUUID, sUUID, 5); err == nil {
					count = live.ActiveShoppers
				}
			}
		}
	}
	envelope := map[string]any{
		"json": count,
	}
	bytes, _ := json.Marshal(envelope)
	select {
	case client.send <- bytes:
	default:
	}

	h.register <- client
	go client.writePump()
	go client.readPump()
}

// ServeLiveEvents handles GET /live/events/{shopId} WebSocket upgrade.
func (h *WebSocketHub) ServeLiveEvents(w http.ResponseWriter, r *http.Request) {
	shopID, tenantID := extractTargetIDs(r)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade events failed: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		tenantID: tenantID,
		shopID:   shopID,
		topic:    "events",
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

// ServeLiveNotifications handles GET /live/notifications/{shopId} WebSocket upgrade.
func (h *WebSocketHub) ServeLiveNotifications(w http.ResponseWriter, r *http.Request) {
	shopID, tenantID := extractTargetIDs(r)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade notifications failed: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		tenantID: tenantID,
		shopID:   shopID,
		topic:    "notifications",
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

// ServeLiveOrganization handles GET /live/organization/{tenantId} WebSocket upgrade.
func (h *WebSocketHub) ServeLiveOrganization(w http.ResponseWriter, r *http.Request) {
	_, tenantID := extractTargetIDs(r)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade organization failed: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		tenantID: tenantID,
		shopID:   tenantID,
		topic:    "organization",
	}
	h.register <- client

	go client.writePump()
	go client.readPump()
}
