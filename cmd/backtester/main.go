package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"global-backtester/internal/backtest"
	"global-backtester/internal/data/stooq"
	"global-backtester/internal/marketdata"
	"global-backtester/internal/strategy"
)

type Market struct {
	Name    string
	Symbol  string
	ISIN    string
	Enabled bool
}

type StrategyRecommendation struct {
	Strategy            string
	BuyCount            int
	SellCount           int
	KeepCount           int
	FinalRecommendation string
	PerSymbol           []SymbolRecommendation
}

type SymbolRecommendation struct {
	Symbol string
	Action string
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
	strategyFlag := flag.String("strategy", "sma", "Strategy to run: sma|buyhold|rsi|all")
	shortFlag := flag.Int("short", 20, "Short moving average window")
	longFlag := flag.Int("long", 100, "Long moving average window")
	rsiPeriodFlag := flag.Int("rsi-period", 14, "RSI period for strategy=rsi")
	rsiOversoldFlag := flag.Float64("rsi-oversold", 30, "RSI oversold threshold for strategy=rsi")
	rsiOverboughtFlag := flag.Float64("rsi-overbought", 70, "RSI overbought threshold for strategy=rsi")
	recommendationTimingFlag := flag.String("recommendation-timing", "next-day-safe", "Recommendation timing: next-day-safe|close")
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
	recommendationTiming, err := normalizeRecommendationTiming(*recommendationTimingFlag)
	fatalIfErr(err, "invalid recommendation timing")
	strategyNames := selectedStrategies(strategyName)
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
	seriesList := make([]marketdata.Series, 0, len(symbols))

	fmt.Printf("Running %s backtest from %s to %s\n\n", displayStrategy(strategyName, *shortFlag, *longFlag, *rsiPeriodFlag, *rsiOversoldFlag, *rsiOverboughtFlag), start.Format("2006-01-02"), end.Format("2006-01-02"))
	fmt.Printf("Recommendation timing mode: %s\n\n", recommendationTiming)

	for _, symbol := range symbols {
		series, err := client.FetchDaily(ctx, symbol, start, end)
		if err != nil {
			fmt.Printf("[WARN] %s skipped: %v\n", symbol, err)
			continue
		}
		seriesList = append(seriesList, series)
	}

	if len(seriesList) == 0 {
		fatal("no symbols were successfully loaded")
	}

	allResults := make(map[string][]backtest.Result, len(strategyNames))
	allRecommendations := make(map[string]StrategyRecommendation, len(strategyNames))
	for _, sName := range strategyNames {
		results := make([]backtest.Result, 0, len(seriesList))
		rec := StrategyRecommendation{Strategy: sName, PerSymbol: make([]SymbolRecommendation, 0, len(seriesList))}
		for _, series := range seriesList {
			signals, err := buildSignals(sName, series.ClosePrices(), *shortFlag, *longFlag, *rsiPeriodFlag, *rsiOversoldFlag, *rsiOverboughtFlag)
			if err != nil {
				fmt.Printf("[WARN] %s (%s) skipped: %v\n", series.Symbol, strings.ToUpper(sName), err)
				continue
			}

			action := recommendationFromSignals(signals, recommendationTiming)
			rec.PerSymbol = append(rec.PerSymbol, SymbolRecommendation{Symbol: series.Symbol, Action: action})
			switch action {
			case "BUY":
				rec.BuyCount++
			case "SELL":
				rec.SellCount++
			default:
				rec.KeepCount++
			}

			result, err := backtest.Run(series, signals, *cashFlag, *feeBpsFlag)
			if err != nil {
				fmt.Printf("[WARN] %s (%s) skipped: %v\n", series.Symbol, strings.ToUpper(sName), err)
				continue
			}
			results = append(results, result)
		}

		if len(results) == 0 {
			fmt.Printf("[WARN] strategy %s produced no valid results\n", strings.ToUpper(sName))
			continue
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].CAGR > results[j].CAGR
		})
		sort.Slice(rec.PerSymbol, func(i, j int) bool {
			return rec.PerSymbol[i].Symbol < rec.PerSymbol[j].Symbol
		})
		rec.FinalRecommendation = finalRecommendation(rec.BuyCount, rec.SellCount, rec.KeepCount)

		allResults[sName] = results
		allRecommendations[sName] = rec
	}

	if len(allResults) == 0 {
		fatal("no symbols were successfully backtested")
	}

	if len(strategyNames) == 1 {
		results := allResults[strategyName]
		printResults(results)
		if rec, ok := allRecommendations[strategyName]; ok {
			printRecommendationSummary(rec, recommendationTiming)
		}
		if *outFlag != "" {
			fatalIfErr(writeCSV(*outFlag, results), "failed writing CSV output")
			fmt.Printf("\nWrote report to %s\n", *outFlag)
		}
		return
	}

	for _, sName := range strategyNames {
		results, ok := allResults[sName]
		if !ok {
			continue
		}
		fmt.Printf("=== %s ===\n", displayStrategy(sName, *shortFlag, *longFlag, *rsiPeriodFlag, *rsiOversoldFlag, *rsiOverboughtFlag))
		printResults(results)
		if rec, ok := allRecommendations[sName]; ok {
			printRecommendationSummary(rec, recommendationTiming)
		}
		fmt.Println()
	}

	printStrategyComparison(allResults, strategyNames)
	if *outFlag != "" {
		fatalIfErr(writeMultiStrategyCSV(*outFlag, allResults, strategyNames), "failed writing multi-strategy CSV output")
		summaryPath := summaryOutputPath(*outFlag)
		fatalIfErr(writeStrategySummaryCSV(summaryPath, allResults, allRecommendations, strategyNames, recommendationTiming), "failed writing strategy-summary CSV output")
		fmt.Printf("\nWrote report to %s\n", *outFlag)
		fmt.Printf("Wrote strategy summary to %s\n", summaryPath)
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

func writeMultiStrategyCSV(path string, allResults map[string][]backtest.Result, strategyNames []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"strategy", "symbol", "start_date", "end_date", "initial_cash", "final_equity", "total_return_pct", "cagr_pct", "max_drawdown_pct", "sharpe", "trades", "in_market_days", "strategy_days"}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, sName := range strategyNames {
		results, ok := allResults[sName]
		if !ok {
			continue
		}
		for _, r := range results {
			row := []string{
				sName,
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
	}

	return w.Error()
}

func writeStrategySummaryCSV(path string, allResults map[string][]backtest.Result, allRecommendations map[string]StrategyRecommendation, strategyNames []string, recommendationTiming string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"strategy", "count", "avg_total_return_pct", "avg_cagr_pct", "avg_max_drawdown_pct", "avg_sharpe", "buy_count", "sell_count", "keep_count", "final_recommendation", "recommendation_timing"}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, sName := range strategyNames {
		results, ok := allResults[sName]
		if !ok || len(results) == 0 {
			continue
		}

		totalReturn := 0.0
		totalCAGR := 0.0
		totalMaxDD := 0.0
		totalSharpe := 0.0
		for _, r := range results {
			totalReturn += r.TotalReturn
			totalCAGR += r.CAGR
			totalMaxDD += r.MaxDrawdown
			totalSharpe += r.Sharpe
		}

		n := float64(len(results))
		rec, hasRec := allRecommendations[sName]
		buyCount := 0
		sellCount := 0
		keepCount := 0
		finalRec := "KEEP"
		if hasRec {
			buyCount = rec.BuyCount
			sellCount = rec.SellCount
			keepCount = rec.KeepCount
			finalRec = rec.FinalRecommendation
		}
		row := []string{
			sName,
			fmt.Sprintf("%d", len(results)),
			fmt.Sprintf("%.4f", (totalReturn/n)*100),
			fmt.Sprintf("%.4f", (totalCAGR/n)*100),
			fmt.Sprintf("%.4f", (totalMaxDD/n)*100),
			fmt.Sprintf("%.4f", totalSharpe/n),
			fmt.Sprintf("%d", buyCount),
			fmt.Sprintf("%d", sellCount),
			fmt.Sprintf("%d", keepCount),
			finalRec,
			recommendationTiming,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return w.Error()
}

func summaryOutputPath(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	if base == "" {
		return path + "_summary.csv"
	}
	if ext == "" {
		return base + "_summary.csv"
	}
	return base + "_summary" + ext
}

func recommendationFromSignals(signals []int, timing string) string {
	if len(signals) == 0 {
		return "KEEP"
	}

	lastIndex := len(signals) - 1
	if timing == "next-day-safe" && len(signals) > 1 {
		lastIndex = len(signals) - 2
	}
	last := signals[lastIndex]
	prev := 0
	if lastIndex > 0 {
		prev = signals[lastIndex-1]
	}

	if last == 1 {
		if prev == 0 {
			return "BUY"
		}
		return "KEEP"
	}

	if prev == 1 {
		return "SELL"
	}

	return "KEEP"
}

func finalRecommendation(buyCount, sellCount, keepCount int) string {
	if buyCount > sellCount && buyCount >= keepCount {
		return "BUY"
	}
	if sellCount > buyCount && sellCount >= keepCount {
		return "SELL"
	}
	return "KEEP"
}

func printRecommendationSummary(rec StrategyRecommendation, timing string) {
	fmt.Printf("\nRecommendation summary (%s):\n", strings.ToUpper(rec.Strategy))
	fmt.Printf("Timing: %s\n", timing)
	fmt.Printf("%-10s %-6s\n", "SYMBOL", "ACTION")
	for _, item := range rec.PerSymbol {
		fmt.Printf("%-10s %-6s\n", item.Symbol, item.Action)
	}
	fmt.Printf("Final recommendation: %s (BUY=%d SELL=%d KEEP=%d)\n", rec.FinalRecommendation, rec.BuyCount, rec.SellCount, rec.KeepCount)
}

func normalizeRecommendationTiming(raw string) (string, error) {
	timing := strings.ToLower(strings.TrimSpace(raw))
	switch timing {
	case "next-day-safe", "nextday", "next-day":
		return "next-day-safe", nil
	case "close", "end-close":
		return "close", nil
	default:
		return "", fmt.Errorf("supported recommendation timing values are: next-day-safe, close")
	}
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

func selectedStrategies(strategyName string) []string {
	if strategyName == "all" {
		return []string{"sma", "buyhold", "rsi"}
	}
	return []string{strategyName}
}

func normalizeStrategyName(raw string) (string, error) {
	strategyName := strings.ToLower(strings.TrimSpace(raw))
	if strategyName == "buy-and-hold" {
		strategyName = "buyhold"
	}
	if strategyName == "compare" {
		strategyName = "all"
	}
	switch strategyName {
	case "sma", "buyhold", "rsi", "all":
		return strategyName, nil
	default:
		return "", fmt.Errorf("supported strategies are: sma, buyhold, rsi, all")
	}
}

func displayStrategy(strategyName string, shortWindow, longWindow, rsiPeriod int, rsiOversold, rsiOverbought float64) string {
	if strategyName == "all" {
		return "ALL STRATEGIES"
	}
	if strategyName == "sma" {
		return fmt.Sprintf("SMA(%d/%d)", shortWindow, longWindow)
	}
	if strategyName == "rsi" {
		return fmt.Sprintf("RSI(period=%d,%.1f/%.1f)", rsiPeriod, rsiOversold, rsiOverbought)
	}
	return "BUYHOLD"
}

func printStrategyComparison(allResults map[string][]backtest.Result, strategyNames []string) {
	fmt.Println("=== Strategy Comparison (Equal-weight averages) ===")
	fmt.Printf("%-14s %8s %10s %10s %10s %8s\n", "STRATEGY", "COUNT", "TOTRET", "CAGR", "MAXDD", "SHARPE")
	for _, sName := range strategyNames {
		results, ok := allResults[sName]
		if !ok || len(results) == 0 {
			continue
		}

		totalReturn := 0.0
		totalCAGR := 0.0
		totalMaxDD := 0.0
		totalSharpe := 0.0
		for _, r := range results {
			totalReturn += r.TotalReturn
			totalCAGR += r.CAGR
			totalMaxDD += r.MaxDrawdown
			totalSharpe += r.Sharpe
		}

		n := float64(len(results))
		avgReturn := totalReturn / n
		avgCAGR := totalCAGR / n
		avgMaxDD := totalMaxDD / n
		avgSharpe := totalSharpe / n

		fmt.Printf("%-14s %8d %9.2f%% %9.2f%% %9.2f%% %8.2f\n",
			strings.ToUpper(sName),
			len(results),
			avgReturn*100,
			avgCAGR*100,
			avgMaxDD*100,
			avgSharpe,
		)
	}
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
