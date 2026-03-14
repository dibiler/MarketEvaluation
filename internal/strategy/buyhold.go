package strategy

import "fmt"

// BuyAndHoldSignals returns a constant long signal after validating input.
func BuyAndHoldSignals(closes []float64) ([]int, error) {
	if len(closes) < 2 {
		return nil, fmt.Errorf("not enough data points (%d) for buy-and-hold", len(closes))
	}

	signals := make([]int, len(closes))
	for i := range signals {
		signals[i] = 1
	}

	return signals, nil
}
