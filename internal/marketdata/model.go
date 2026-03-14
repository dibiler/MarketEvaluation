package marketdata

import "time"

// Candle represents one daily bar of historical market data.
type Candle struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// Series contains all candles for a single symbol.
type Series struct {
	Symbol  string
	Candles []Candle
}

func (s Series) ClosePrices() []float64 {
	prices := make([]float64, 0, len(s.Candles))
	for _, c := range s.Candles {
		prices = append(prices, c.Close)
	}
	return prices
}

func (s Series) Dates() []time.Time {
	dates := make([]time.Time, 0, len(s.Candles))
	for _, c := range s.Candles {
		dates = append(dates, c.Date)
	}
	return dates
}
