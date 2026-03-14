package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"global-backtester/internal/backtest"
	"global-backtester/internal/data/stooq"
	"global-backtester/internal/strategy"
)

func main() {
	defaultEnd := time.Now().Format("2006-01-02")

	symbolsFlag := flag.String("symbols", "SPY.US,VGK.US,EWJ.US,EEM.US,TLT.US,GLD.US", "Comma-separated tickers (Stooq format)")
	startFlag := flag.String("start", "2015-01-01", "Start date YYYY-MM-DD")
	endFlag := flag.String("end", defaultEnd, "End date YYYY-MM-DD")
	cashFlag := flag.Float64("cash", 10000, "Initial cash per symbol")
	shortFlag := flag.Int("short", 20, "Short moving average window")
	longFlag := flag.Int("long", 100, "Long moving average window")
	feeBpsFlag := flag.Float64("fee-bps", 5, "Transaction cost in basis points")
	outFlag := flag.String("out", "", "Optional CSV output path")
	flag.Parse()

	start, err := time.Parse("2006-01-02", *startFlag)
	fatalIfErr(err, "invalid start date")
	end, err := time.Parse("2006-01-02", *endFlag)
	fatalIfErr(err, "invalid end date")
	if end.Before(start) {
		fatal("end date must be after start date")
	}
	if *shortFlag >= *longFlag {
		fatal("short window must be less than long window")
	}

	symbols := splitSymbols(*symbolsFlag)
	if len(symbols) == 0 {
		fatal("at least one symbol is required")
	}

	ctx := context.Background()
	client := stooq.NewClient()
	results := make([]backtest.Result, 0, len(symbols))

	fmt.Printf("Running SMA(%d/%d) backtest from %s to %s\n\n", *shortFlag, *longFlag, start.Format("2006-01-02"), end.Format("2006-01-02"))

	for _, symbol := range symbols {
		series, err := client.FetchDaily(ctx, symbol, start, end)
		if err != nil {
			fmt.Printf("[WARN] %s skipped: %v\n", symbol, err)
			continue
		}

		signals, err := strategy.SMACrossoverSignals(series.ClosePrices(), *shortFlag, *longFlag)
		if err != nil {
			fmt.Printf("[WARN] %s skipped: %v\n", symbol, err)
			continue
		}

		result, err := backtest.Run(series, signals, *cashFlag, *feeBpsFlag)
		if err != nil {
			fmt.Printf("[WARN] %s skipped: %v\n", symbol, err)
			continue
		}
		results = append(results, result)
	}

	if len(results) == 0 {
		fatal("no symbols were successfully backtested")
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CAGR > results[j].CAGR
	})

	printResults(results)
	if *outFlag != "" {
		fatalIfErr(writeCSV(*outFlag, results), "failed writing CSV output")
		fmt.Printf("\nWrote report to %s\n", *outFlag)
	}
}

func printResults(results []backtest.Result) {
	fmt.Printf("%-10s %12s %10s %10s %10s %8s %8s\n", "SYMBOL", "FINAL", "TOTRET", "CAGR", "MAXDD", "SHARPE", "TRADES")
	for _, r := range results {
		fmt.Printf("%-10s %12.2f %9.2f%% %9.2f%% %9.2f%% %8.2f %8d\n",
			r.Symbol,
			r.FinalEquity,
			r.TotalReturn*100,
			r.CAGR*100,
			r.MaxDrawdown*100,
			r.Sharpe,
			r.TradeCount,
		)
	}

	portfolioCAGR := 0.0
	portfolioReturn := 0.0
	for _, r := range results {
		portfolioCAGR += r.CAGR
		portfolioReturn += r.TotalReturn
	}
	portfolioCAGR /= float64(len(results))
	portfolioReturn /= float64(len(results))

	fmt.Printf("\nEqual-weight average return: %.2f%%\n", portfolioReturn*100)
	fmt.Printf("Equal-weight average CAGR:   %.2f%%\n", portfolioCAGR*100)
}

func writeCSV(path string, results []backtest.Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"symbol", "start_date", "end_date", "initial_cash", "final_equity", "total_return_pct", "cagr_pct", "max_drawdown_pct", "sharpe", "trades", "in_market_days", "strategy_days"}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range results {
		row := []string{
			r.Symbol,
			r.StartDate.Format("2006-01-02"),
			r.EndDate.Format("2006-01-02"),
			fmt.Sprintf("%.2f", r.InitialCash),
			fmt.Sprintf("%.2f", r.FinalEquity),
			fmt.Sprintf("%.4f", r.TotalReturn*100),
			fmt.Sprintf("%.4f", r.CAGR*100),
			fmt.Sprintf("%.4f", r.MaxDrawdown*100),
			fmt.Sprintf("%.4f", r.Sharpe),
			fmt.Sprintf("%d", r.TradeCount),
			fmt.Sprintf("%d", r.InMarketDays),
			fmt.Sprintf("%d", r.StrategyDays),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return w.Error()
}

func splitSymbols(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func fatalIfErr(err error, msg string) {
	if err != nil {
		fatal(msg + ": " + err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}
