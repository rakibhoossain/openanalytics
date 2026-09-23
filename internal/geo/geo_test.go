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
