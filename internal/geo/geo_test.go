package geo

import (
	"testing"
)

func TestDatacenterASNClassification(t *testing.T) {
	tests := []struct {
		asn          uint
		isDatacenter bool
	}{
		{asn: 16509, isDatacenter: true}, // AWS
		{asn: 15169, isDatacenter: true}, // Google
		{asn: 13335, isDatacenter: true}, // Cloudflare
		{asn: 7922, isDatacenter: false}, // Comcast (Residential ISP)
		{asn: 7018, isDatacenter: false}, // AT&T
	}

	for _, tt := range tests {
		got := isKnownDatacenterASN(tt.asn)
		if got != tt.isDatacenter {
			t.Errorf("isKnownDatacenterASN(%d) = %v; want %v", tt.asn, got, tt.isDatacenter)
		}
	}
}

func TestNewServiceMissingDir(t *testing.T) {
	tempDir := t.TempDir()
	svc, err := NewService(Config{DataDir: tempDir})
	if err != nil {
		t.Fatalf("unexpected error initializing service: %v", err)
	}
	defer svc.Close()

	// Should return friendly error awaiting download when database file does not exist yet
	_, err = svc.Lookup("8.8.8.8")
	if err == nil {
		t.Errorf("expected error when database is missing")
	}
}

func TestRealGeoDatabaseLookup(t *testing.T) {
	svc, err := NewService(Config{DataDir: "../../data/geo"})
	if err != nil {
		t.Skip("skipping real database test if files not present")
	}
	defer svc.Close()

	loc, err := svc.Lookup("8.8.8.8")
	if err != nil {
		t.Fatalf("failed to lookup 8.8.8.8: %v", err)
	}
	if loc.Country != "US" {
		t.Errorf("expected country US, got %s", loc.Country)
	}

	asn, err := svc.LookupASN("8.8.8.8")
	if err != nil {
		t.Fatalf("failed to lookup ASN for 8.8.8.8: %v", err)
	}
	if asn.AutonomousSystemNumber != 15169 {
		t.Errorf("expected ASN 15169 (Google), got %d", asn.AutonomousSystemNumber)
	}
	if !asn.IsDatacenter {
		t.Errorf("expected Google 15169 to be classified as datacenter")
	}
}

