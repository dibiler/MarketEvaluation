package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"global-backtester/internal/backtest"
	"global-backtester/internal/data/stooq"
	"global-backtester/internal/strategy"
)

type Market struct {
	Name    string
	Symbol  string
	ISIN    string
	Enabled bool
}

func main() {
	defaultEnd := time.Now().Format("2006-01-02")

	symbolsFlag := flag.String("symbols", "SPY.US,VGK.US,EWJ.US,EEM.US,TLT.US,GLD.US", "Comma-separated tickers (Stooq format)")
	marketsFileFlag := flag.String("markets-file", "markets.csv", "Path to markets catalog CSV (name,symbol,isin,enabled)")
	useMarketsFlag := flag.Bool("use-markets", false, "Use symbols from -markets-file")
	isinsFlag := flag.String("isins", "", "Optional comma-separated ISIN filter when using -use-markets")
	startFlag := flag.String("start", "2015-01-01", "Start date YYYY-MM-DD")
	endFlag := flag.String("end", defaultEnd, "End date YYYY-MM-DD")
	cashFlag := flag.Float64("cash", 10000, "Initial cash per symbol")
	strategyFlag := flag.String("strategy", "sma", "Strategy to run: sma|buyhold")
	shortFlag := flag.Int("short", 20, "Short moving average window")
	longFlag := flag.Int("long", 100, "Long moving average window")
	rsiPeriodFlag := flag.Int("rsi-period", 14, "RSI period for strategy=rsi")
	rsiOversoldFlag := flag.Float64("rsi-oversold", 30, "RSI oversold threshold for strategy=rsi")
	rsiOverboughtFlag := flag.Float64("rsi-overbought", 70, "RSI overbought threshold for strategy=rsi")
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
	strategyName, err := normalizeStrategyName(*strategyFlag)
	fatalIfErr(err, "invalid strategy")
	if strategyName == "sma" && *shortFlag >= *longFlag {
		fatal("short window must be less than long window")
	}

	symbols := splitSymbols(*symbolsFlag)
	if *useMarketsFlag {
		markets, err := loadMarkets(*marketsFileFlag)
		fatalIfErr(err, "failed loading markets file")

		symbols, err = symbolsFromMarkets(markets, splitSymbols(*isinsFlag))
		fatalIfErr(err, "failed selecting symbols from markets file")

		fmt.Printf("Loaded %d symbols from %s\n", len(symbols), *marketsFileFlag)
	}
	if len(symbols) == 0 {
		fatal("at least one symbol is required")
	}

	ctx := context.Background()
	client := stooq.NewClient()
	results := make([]backtest.Result, 0, len(symbols))

	fmt.Printf("Running %s backtest from %s to %s\n\n", displayStrategy(strategyName, *shortFlag, *longFlag, *rsiPeriodFlag, *rsiOversoldFlag, *rsiOverboughtFlag), start.Format("2006-01-02"), end.Format("2006-01-02"))

	for _, symbol := range symbols {
		series, err := client.FetchDaily(ctx, symbol, start, end)
		if err != nil {
			fmt.Printf("[WARN] %s skipped: %v\n", symbol, err)
			continue
		}

		signals, err := buildSignals(strategyName, series.ClosePrices(), *shortFlag, *longFlag, *rsiPeriodFlag, *rsiOversoldFlag, *rsiOverboughtFlag)
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

func buildSignals(strategyName string, closes []float64, shortWindow, longWindow, rsiPeriod int, rsiOversold, rsiOverbought float64) ([]int, error) {
	switch strategyName {
	case "sma":
		return strategy.SMACrossoverSignals(closes, shortWindow, longWindow)
	case "buyhold":
		return strategy.BuyAndHoldSignals(closes)
	case "rsi":
		return strategy.RSIMeanReversionSignals(closes, rsiPeriod, rsiOversold, rsiOverbought)
	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategyName)
	}
}

func normalizeStrategyName(raw string) (string, error) {
	strategyName := strings.ToLower(strings.TrimSpace(raw))
	if strategyName == "buy-and-hold" {
		strategyName = "buyhold"
	}
	switch strategyName {
	case "sma", "buyhold", "rsi":
		return strategyName, nil
	default:
		return "", fmt.Errorf("supported strategies are: sma, buyhold, rsi")
	}
}

func displayStrategy(strategyName string, shortWindow, longWindow, rsiPeriod int, rsiOversold, rsiOverbought float64) string {
	if strategyName == "sma" {
		return fmt.Sprintf("SMA(%d/%d)", shortWindow, longWindow)
	}
	if strategyName == "rsi" {
		return fmt.Sprintf("RSI(period=%d,%.1f/%.1f)", rsiPeriod, rsiOversold, rsiOverbought)
	}
	return "BUYHOLD"
}

func loadMarkets(path string) ([]Market, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	markets := make([]Market, 0, 64)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) < 3 {
			continue
		}

		name := strings.TrimSpace(record[0])
		symbol := strings.TrimSpace(record[1])
		isin := strings.ToUpper(strings.TrimSpace(record[2]))
		if name == "" && symbol == "" && isin == "" {
			continue
		}

		if strings.EqualFold(name, "name") && strings.EqualFold(symbol, "symbol") {
			continue
		}

		enabled := true
		if len(record) >= 4 {
			rawEnabled := strings.TrimSpace(record[3])
			if rawEnabled != "" {
				parsed, parseErr := strconv.ParseBool(rawEnabled)
				if parseErr == nil {
					enabled = parsed
				} else {
					rawLower := strings.ToLower(rawEnabled)
					enabled = rawLower == "1" || rawLower == "y" || rawLower == "yes"
				}
			}
		}

		if symbol == "" || isin == "" {
			continue
		}

		markets = append(markets, Market{
			Name:    name,
			Symbol:  symbol,
			ISIN:    isin,
			Enabled: enabled,
		})
	}

	if len(markets) == 0 {
		return nil, fmt.Errorf("no valid market entries found in %s", path)
	}

	return markets, nil
}

func symbolsFromMarkets(markets []Market, isins []string) ([]string, error) {
	allowedISIN := make(map[string]struct{}, len(isins))
	for _, isin := range isins {
		normalized := strings.ToUpper(strings.TrimSpace(isin))
		if normalized != "" {
			allowedISIN[normalized] = struct{}{}
		}
	}
	useISINFilter := len(allowedISIN) > 0

	uniq := make(map[string]struct{})
	selected := make([]string, 0, len(markets))
	for _, m := range markets {
		if useISINFilter {
			if _, ok := allowedISIN[m.ISIN]; !ok {
				continue
			}
		} else if !m.Enabled {
			continue
		}
		if _, exists := uniq[m.Symbol]; exists {
			continue
		}
		uniq[m.Symbol] = struct{}{}
		selected = append(selected, m.Symbol)
	}

	if len(selected) == 0 {
		if useISINFilter {
			return nil, fmt.Errorf("no markets matched the provided ISIN filter")
		}
		return nil, fmt.Errorf("no enabled markets found")
	}

	return selected, nil
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
