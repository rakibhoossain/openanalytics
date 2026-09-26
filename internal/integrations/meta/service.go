package meta

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"openanalytics/internal/domain"
)

// Service coordinates event mapping, credential resolution, and asynchronous dispatch to Meta CAPI.
type Service struct {
	repo       *Repository
	client     *Client
	eventQueue chan *domain.Event
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewService creates a Meta CAPI forwarder with an asynchronous worker pool.
func NewService(ctx context.Context, repo *Repository, client *Client) *Service {
	if client == nil {
		client = NewClient("")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	s := &Service{
		repo:       repo,
		client:     client,
		eventQueue: make(chan *domain.Event, 2048),
		ctx:        workerCtx,
		cancel:     cancel,
	}

	// Launch 4 concurrent forwarder worker goroutines
	for i := 0; i < 4; i++ {
		s.wg.Add(1)
		go s.workerLoop(i)
	}

	return s
}

// DispatchAsync enqueues an event for asynchronous forwarding to Meta CAPI without blocking the caller.
func (s *Service) DispatchAsync(event *domain.Event) {
	if s == nil || event == nil {
		return
	}
	select {
	case s.eventQueue <- event:
	default:
		log.Printf("[Meta CAPI] Warning: Event queue full, dropped async dispatch for event %s (Shop: %s)",
			event.Name, event.ShopID.String())
	}
}

// ProcessEvent synchronously evaluates and forwards a single domain event to Meta CAPI.
func (s *Service) ProcessEvent(ctx context.Context, event *domain.Event) error {
	if s.repo == nil || s.client == nil || event == nil {
		return nil
	}

	// 1. Resolve shop integration configuration
	integration, err := s.repo.GetMetaIntegration(ctx, event.ShopID)
	if err != nil {
		return err
	}
	if integration == nil || !integration.Enabled {
		return nil // Meta integration not configured or disabled for this shop
	}

	creds := integration.Credentials
	if creds.PixelID == "" || creds.AccessToken == "" {
		return nil
	}

	// 2. Map domain event to Meta CAPI event
	capiEvent, ok := MapToMetaEvent(event)
	if !ok {
		return nil // Not a standard Meta event
	}

	// 3. Filter against merchant's enabled event whitelist (if configured)
	if len(integration.EventsWhitelist) > 0 {
		isWhitelisted := false
		for _, w := range integration.EventsWhitelist {
			if strings.EqualFold(w, capiEvent.EventName) {
				isWhitelisted = true
				break
			}
		}
		if !isWhitelisted {
			return nil // Event is disabled in merchant settings
		}
	}

	// 4. Send to Meta Conversions API
	resp, err := s.client.SendEvents(ctx, creds.PixelID, creds.AccessToken, creds.TestEventCode, []CAPIEvent{*capiEvent})
	if err != nil {
		log.Printf("[Meta CAPI Error] Failed to dispatch %s for shop %s: %v",
			capiEvent.EventName, event.ShopID.String(), err)
		return err
	}

	if resp != nil && resp.EventsReceived > 0 {
		log.Printf("[Meta CAPI Success] Delivered %s for shop %s (Received: %d, Trace: %s, EventID: %s)",
			capiEvent.EventName, event.ShopID.String(), resp.EventsReceived, resp.FBTraceID, capiEvent.EventID)
	}

	return nil
}

func (s *Service) workerLoop(workerID int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-s.eventQueue:
			if !ok {
				return
			}
			timeoutCtx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			_ = s.ProcessEvent(timeoutCtx, event)
			cancel()
		}
	}
}

// Close gracefully terminates all worker loops.
func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}
