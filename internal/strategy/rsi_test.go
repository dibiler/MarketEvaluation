package strategy

import "testing"

func TestRSIMeanReversionSignals(t *testing.T) {
	closes := []float64{100, 99, 98, 97, 96, 95, 96, 97, 98, 99, 100, 101, 102, 103}
	signals, err := RSIMeanReversionSignals(closes, 5, 35, 65)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signals) != len(closes) {
		t.Fatalf("expected %d signals, got %d", len(closes), len(signals))
	}

	foundLong := false
	for i := range signals {
		if signals[i] == 1 {
			foundLong = true
			break
		}
	}
	if !foundLong {
		t.Fatalf("expected at least one long signal")
	}
}

func TestRSIMeanReversionSignalsBadInputs(t *testing.T) {
	_, err := RSIMeanReversionSignals([]float64{100, 101}, 5, 30, 70)
	if err == nil {
		t.Fatalf("expected error for insufficient data")
	}

	_, err = RSIMeanReversionSignals([]float64{100, 101, 102, 103, 104, 105}, 5, 80, 70)
	if err == nil {
		t.Fatalf("expected error for invalid thresholds")
	}
}
