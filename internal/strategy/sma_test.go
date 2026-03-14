package strategy

import "testing"

func TestSMACrossoverSignals(t *testing.T) {
	closes := []float64{10, 10, 10, 10, 11, 12, 13, 14}
	signals, err := SMACrossoverSignals(closes, 2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != len(closes) {
		t.Fatalf("expected %d signals, got %d", len(closes), len(signals))
	}

	if signals[3] != 0 {
		t.Fatalf("expected warm-up signal to be 0, got %d", signals[3])
	}
	if signals[6] != 1 {
		t.Fatalf("expected trend-following signal to be 1 at index 6, got %d", signals[6])
	}
}
