package backtest

import (
	"fmt"
	"math"
	"time"

	"global-backtester/internal/marketdata"
)

type Result struct {
	Symbol       string
	StartDate    time.Time
	EndDate      time.Time
	InitialCash  float64
	FinalEquity  float64
	TotalReturn  float64
	CAGR         float64
	MaxDrawdown  float64
	Sharpe       float64
	TradeCount   int
	InMarketDays int
	StrategyDays int
}

// Run executes a long-only backtest using prior-day signal for next-day return.
func Run(series marketdata.Series, signals []int, initialCash, feeBps float64) (Result, error) {
	if len(series.Candles) < 2 {
		return Result{}, fmt.Errorf("series for %s has fewer than 2 candles", series.Symbol)
	}
	if len(signals) != len(series.Candles) {
		return Result{}, fmt.Errorf("signals length (%d) does not match candles length (%d)", len(signals), len(series.Candles))
	}
	if initialCash <= 0 {
		return Result{}, fmt.Errorf("initial cash must be > 0")
	}

	equity := initialCash
	equityCurve := make([]float64, len(series.Candles))
	equityCurve[0] = equity

	strategyReturns := make([]float64, 0, len(series.Candles)-1)
	trades := 0
	inMarketDays := 0
	prevPosition := 0
	feeRate := feeBps / 10000.0

	for i := 1; i < len(series.Candles); i++ {
		position := signals[i-1]
		if position == 1 {
			inMarketDays++
		}

		if position != prevPosition && i > 1 {
			equity *= (1.0 - feeRate)
			trades++
		}

		prevClose := series.Candles[i-1].Close
		currClose := series.Candles[i].Close
		if prevClose <= 0 || currClose <= 0 {
			return Result{}, fmt.Errorf("invalid close price for %s on %s", series.Symbol, series.Candles[i].Date.Format("2006-01-02"))
		}

		ret := currClose/prevClose - 1.0
		strategyRet := 0.0
		if position == 1 {
			strategyRet = ret
		}

		equity *= (1.0 + strategyRet)
		equityCurve[i] = equity
		strategyReturns = append(strategyReturns, strategyRet)
	}

	totalReturn := equity/initialCash - 1.0
	startDate := series.Candles[0].Date
	endDate := series.Candles[len(series.Candles)-1].Date
	years := endDate.Sub(startDate).Hours() / (24.0 * 365.25)
	cagr := 0.0
	if years > 0 {
		cagr = math.Pow(equity/initialCash, 1.0/years) - 1.0
	}

	maxDD := maxDrawdown(equityCurve)
	sharpe := annualizedSharpe(strategyReturns)

	return Result{
		Symbol:       series.Symbol,
		StartDate:    startDate,
		EndDate:      endDate,
		InitialCash:  initialCash,
		FinalEquity:  equity,
		TotalReturn:  totalReturn,
		CAGR:         cagr,
		MaxDrawdown:  maxDD,
		Sharpe:       sharpe,
		TradeCount:   trades,
		InMarketDays: inMarketDays,
		StrategyDays: len(strategyReturns),
	}, nil
}

func annualizedSharpe(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}

	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	varSum := 0.0
	for _, r := range returns {
		d := r - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(len(returns)-1))
	if std == 0 {
		return 0
	}

	return (mean / std) * math.Sqrt(252)
}

func maxDrawdown(curve []float64) float64 {
	if len(curve) == 0 {
		return 0
	}
	peak := curve[0]
	maxDD := 0.0
	for _, v := range curve {
		if v > peak {
			peak = v
		}
		dd := (v / peak) - 1.0
		if dd < maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
