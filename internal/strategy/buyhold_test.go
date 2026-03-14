package strategy

import "testing"

func TestBuyAndHoldSignals(t *testing.T) {
	signals, err := BuyAndHoldSignals([]float64{100, 101, 102})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != 3 {
		t.Fatalf("expected 3 signals, got %d", len(signals))
	}
	for i, s := range signals {
		if s != 1 {
			t.Fatalf("expected signal 1 at index %d, got %d", i, s)
		}
	}
}

func TestBuyAndHoldSignalsTooShort(t *testing.T) {
	_, err := BuyAndHoldSignals([]float64{100})
	if err == nil {
		t.Fatalf("expected error for too-short input")
	}
}
