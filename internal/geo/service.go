package geo

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

const (
	DefaultCityDB = "GeoLite2-City.mmdb"
	DefaultASNDB  = "GeoLite2-ASN.mmdb"
	DefaultWatchInterval = 1 * time.Hour
)

// Service manages GeoIP lookups with thread-safe hot-reloading from disk.
type Service struct {
	mu         sync.RWMutex
	cityReader *geoip2.Reader
	asnReader  *geoip2.Reader

	dataDir         string
	cityPath        string
	asnPath         string
	lastCityModTime time.Time
	lastASNModTime  time.Time
}

// Config holds options for the Geo Service.
type Config struct {
	DataDir string // Path to directory containing .mmdb files (e.g. "data/geo")
}

// NewService initializes the Geo Service, loading existing databases from dataDir.
func NewService(cfg Config) (*Service, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = "data/geo"
	}

	cityPath := filepath.Join(cfg.DataDir, DefaultCityDB)
	asnPath := filepath.Join(cfg.DataDir, DefaultASNDB)

	s := &Service{
		dataDir:  cfg.DataDir,
		cityPath: cityPath,
		asnPath:  asnPath,
	}

	// Initial load from disk
	if err := s.reload(); err != nil {
		log.Printf("[GeoIP] Notice: Geo databases not yet loaded from %s: %v", cfg.DataDir, err)
	}

	return s, nil
}

// StartWatcher periodically checks if the .mmdb files on disk have been updated (e.g. by geoipupdate container).
func (s *Service) StartWatcher(ctx context.Context, checkInterval time.Duration) {
	if checkInterval == 0 {
		checkInterval = DefaultWatchInterval
	}

	ticker := time.NewTicker(checkInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.hasFileChanged() {
					log.Println("[GeoIP] New database files detected on disk, hot-reloading...")
					if err := s.reload(); err != nil {
						log.Printf("[GeoIP] Hot-reload error: %v", err)
					} else {
						log.Println("[GeoIP] Databases successfully hot-reloaded into memory")
					}
				}
			}
		}
	}()
}

func (s *Service) hasFileChanged() bool {
	if info, err := os.Stat(s.cityPath); err == nil {
		if info.ModTime().After(s.lastCityModTime) {
			return true
		}
	}
	if info, err := os.Stat(s.asnPath); err == nil {
		if info.ModTime().After(s.lastASNModTime) {
			return true
		}
	}
	return false
}

func (s *Service) reload() error {
	var newCityReader *geoip2.Reader
	var newASNReader *geoip2.Reader
	var cityMod, asnMod time.Time

	if info, err := os.Stat(s.cityPath); err == nil {
		reader, err := geoip2.Open(s.cityPath)
		if err != nil {
			return fmt.Errorf("failed to open city database %s: %w", s.cityPath, err)
		}
		newCityReader = reader
		cityMod = info.ModTime()
	}

	if info, err := os.Stat(s.asnPath); err == nil {
		reader, err := geoip2.Open(s.asnPath)
		if err == nil {
			newASNReader = reader
			asnMod = info.ModTime()
		}
	}

	if newCityReader == nil && newASNReader == nil {
		return fmt.Errorf("no geoip databases found in %s", s.dataDir)
	}

	s.mu.Lock()
	oldCity := s.cityReader
	oldASN := s.asnReader
	if newCityReader != nil {
		s.cityReader = newCityReader
		s.lastCityModTime = cityMod
	}
	if newASNReader != nil {
		s.asnReader = newASNReader
		s.lastASNModTime = asnMod
	}
	s.mu.Unlock()

	// Safely close replaced readers
	if oldCity != nil && newCityReader != nil {
		_ = oldCity.Close()
	}
	if oldASN != nil && newASNReader != nil {
		_ = oldASN.Close()
	}

	return nil
}

// Lookup resolves geographic location from an IP string.
func (s *Service) Lookup(ipStr string) (*Location, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	s.mu.RLock()
	reader := s.cityReader
	s.mu.RUnlock()

	if reader == nil {
		return nil, fmt.Errorf("city database not initialized (awaiting download into %s)", s.dataDir)
	}

	record, err := reader.City(ip)
	if err != nil {
		return nil, err
	}

	loc := &Location{
		Country:  record.Country.IsoCode,
		City:     record.City.Names["en"],
		Timezone: record.Location.TimeZone,
	}

	if len(record.Subdivisions) > 0 {
		loc.Region = record.Subdivisions[0].IsoCode
	}

	if record.Location.Latitude != 0 || record.Location.Longitude != 0 {
		lat := float32(record.Location.Latitude)
		lon := float32(record.Location.Longitude)
		loc.Latitude = &lat
		loc.Longitude = &lon
	}

	return loc, nil
}

// LookupASN resolves Autonomous System metadata from an IP string.
func (s *Service) LookupASN(ipStr string) (*ASNInfo, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	s.mu.RLock()
	reader := s.asnReader
	s.mu.RUnlock()

	if reader == nil {
		return nil, fmt.Errorf("asn database not initialized (awaiting download into %s)", s.dataDir)
	}

	record, err := reader.ASN(ip)
	if err != nil {
		return nil, err
	}

	info := &ASNInfo{
		AutonomousSystemNumber:       record.AutonomousSystemNumber,
		AutonomousSystemOrganization: record.AutonomousSystemOrganization,
		IsDatacenter:                 isKnownDatacenterASN(record.AutonomousSystemNumber),
	}

	return info, nil
}

// Close gracefully closes open database readers.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cityReader != nil {
		_ = s.cityReader.Close()
		s.cityReader = nil
	}
	if s.asnReader != nil {
		_ = s.asnReader.Close()
		s.asnReader = nil
	}
	return nil
}

// isKnownDatacenterASN checks against prominent cloud/hosting ASNs (AWS, GCP, Azure, Cloudflare, DigitalOcean, Hetzner, etc.).
func isKnownDatacenterASN(asn uint) bool {
	switch asn {
	case 16509, 14618, 8075, 15169, 13335, 14061, 24940, 20473, 63949, 396982:
		return true
	default:
		return false
	}
}
