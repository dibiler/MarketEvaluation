package strategy

import "fmt"

// SMACrossoverSignals returns 1 when short MA is above long MA, otherwise 0.
func SMACrossoverSignals(closes []float64, shortWindow, longWindow int) ([]int, error) {
	if shortWindow < 1 || longWindow < 1 {
		return nil, fmt.Errorf("windows must be positive")
	}
	if shortWindow >= longWindow {
		return nil, fmt.Errorf("short window must be less than long window")
	}
	if len(closes) < longWindow {
		return nil, fmt.Errorf("not enough data points (%d) for long window (%d)", len(closes), longWindow)
	}

	signals := make([]int, len(closes))
	shortSum := 0.0
	longSum := 0.0

	for i := 0; i < len(closes); i++ {
		close := closes[i]
		shortSum += close
		longSum += close

		if i >= shortWindow {
			shortSum -= closes[i-shortWindow]
		}
		if i >= longWindow {
			longSum -= closes[i-longWindow]
		}

		if i < longWindow-1 {
			signals[i] = 0
			continue
		}

		shortMA := shortSum / float64(shortWindow)
		longMA := longSum / float64(longWindow)
		if shortMA > longMA {
			signals[i] = 1
		}
	}

	return signals, nil
}
