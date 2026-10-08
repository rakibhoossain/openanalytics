package currency

import (
	"testing"
)

func TestCurrencyServiceConversion(t *testing.T) {
	s := NewService(nil, nil)
	s.SetRates(map[string]float64{
		"USD": 1.0,
		"BDT": 120.0,
		"EUR": 0.89,
	})

	// 1. Transaction in BDT -> Base USD
	// 12000 BDT cents / 120.0 = 100 USD cents
	usdCents := s.ConvertToUSD(12000, "BDT")
	if usdCents != 100 {
		t.Errorf("ConvertToUSD(12000, 'BDT') = %d, want 100", usdCents)
	}

	// 2. Base USD -> Shop Currency in EUR
	// 100 USD cents * 0.89 = 89 EUR cents
	eurCents := s.ConvertFromUSD(usdCents, "EUR")
	if eurCents != 89 {
		t.Errorf("ConvertFromUSD(100, 'EUR') = %d, want 89", eurCents)
	}

	// 3. Fallback when currency unknown defaults to rate 1.0
	unknownCents := s.ConvertToUSD(500, "XYZ")
	if unknownCents != 500 {
		t.Errorf("ConvertToUSD(500, 'XYZ') = %d, want 500", unknownCents)
	}

	// 4. Rate lookup
	if rate := s.GetRate("bdt"); rate != 120.0 {
		t.Errorf("GetRate('bdt') = %f, want 120.0", rate)
	}
}
