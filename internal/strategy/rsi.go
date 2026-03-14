package strategy

import "fmt"

// RSIMeanReversionSignals returns long-only signals based on RSI levels.
// Signal behavior:
// - Enter long when RSI < oversold
// - Exit to cash when RSI > overbought
// - Otherwise keep previous position
func RSIMeanReversionSignals(closes []float64, period int, oversold, overbought float64) ([]int, error) {
	if len(closes) < period+1 {
		return nil, fmt.Errorf("not enough data points (%d) for RSI period (%d)", len(closes), period)
	}
	if period < 2 {
		return nil, fmt.Errorf("rsi period must be >= 2")
	}
	if oversold <= 0 || overbought >= 100 || oversold >= overbought {
		return nil, fmt.Errorf("invalid RSI thresholds: oversold must be > 0, overbought < 100, and oversold < overbought")
	}

	signals := make([]int, len(closes))
	avgGain := 0.0
	avgLoss := 0.0

	for i := 1; i <= period; i++ {
		delta := closes[i] - closes[i-1]
		if delta > 0 {
			avgGain += delta
		} else {
			avgLoss -= delta
		}
	}

	avgGain /= float64(period)
	avgLoss /= float64(period)
	position := 0

	for i := period; i < len(closes); i++ {
		if i > period {
			delta := closes[i] - closes[i-1]
			gain := 0.0
			loss := 0.0
			if delta > 0 {
				gain = delta
			} else {
				loss = -delta
			}
			avgGain = ((avgGain * float64(period-1)) + gain) / float64(period)
			avgLoss = ((avgLoss * float64(period-1)) + loss) / float64(period)
		}

		rsi := 100.0
		if avgLoss > 0 {
			rs := avgGain / avgLoss
			rsi = 100.0 - (100.0 / (1.0 + rs))
		}

		if rsi < oversold {
			position = 1
		} else if rsi > overbought {
			position = 0
		}

		signals[i] = position
	}

	return signals, nil
}
