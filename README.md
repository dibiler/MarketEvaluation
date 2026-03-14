# Global Market Strategy Backtester (Go)

A CLI project for retrieving global market historical data and testing investing strategies.

## What's new

- Multi-strategy runs with `-strategy all` (SMA, BUYHOLD, RSI in one execution).
- Recommendation output per symbol and per strategy: `BUY`, `SELL`, `KEEP`.
- Default recommendation timing is now `next-day-safe`.
- Multi-strategy CSV exports:
  - detailed rows in your `-out` file
  - aggregated strategy summary in `*_summary.csv`
- End date defaults to the current date when `-end` is omitted.

Quick examples:

```bash
# Run all strategies and export both detailed + summary CSVs
go run ./cmd/backtester \
  -strategy all \
  -symbols "SPY.US,VGK.US,EWJ.US,GLD.US" \
  -start "2018-01-01" \
  -out compare.csv

# Override recommendation timing to end-of-close mode
go run ./cmd/backtester \
  -strategy all \
  -symbols "SPY.US,GLD.US" \
  -start "2024-01-01" \
  -recommendation-timing close
```

## What it does

- Downloads daily historical candles from Stooq (free global market source).
- Runs multiple long-only strategies (SMA crossover, buy-and-hold, and RSI mean reversion).
- Computes core backtest metrics:
  - Total return
  - CAGR
  - Max drawdown
  - Sharpe ratio
  - Trade count
- Supports running multiple market symbols in one command.

## Strategies implemented

This project currently implements three trading strategies.

### 1) SMA crossover (trend-following)

Where implemented:

- Signal generation: internal/strategy/sma.go
- Portfolio simulation and metrics: internal/backtest/engine.go

Idea:

- Use two simple moving averages on daily close prices.
- If short SMA is above long SMA, the strategy is in the market (position = 1).
- Otherwise, it is out of the market (position = 0, cash).

Signal rules:

- Inputs:
  - short window: command flag -short
  - long window: command flag -long
- Constraints:
  - both windows must be positive
  - short window must be less than long window
  - enough history is required to compute the long window
- Warm-up period:
  - before the long window is available, signals are 0

Execution model in this app:

- Long-only, no short selling.
- Uses prior-day signal for next-day return.
- If signal at day t-1 is 1, day t market return is applied.
- If signal is 0, day t return is 0 (stay in cash).
- A transaction fee is charged whenever position changes (flag -fee-bps).

Backtest outputs for this strategy:

- Final equity
- Total return
- CAGR
- Max drawdown
- Sharpe ratio (annualized with 252 trading days)
- Trade count
- In-market days

### 2) Buy-and-hold benchmark

Where implemented:

- Signal generation: internal/strategy/buyhold.go
- Portfolio simulation and metrics: internal/backtest/engine.go

Idea:

- Stay invested for the whole test period.
- Every signal is 1, so the system captures each daily market move.

Signal rules:

- Requires at least two close prices.
- No moving-average parameters are used.

Execution model in this app:

- Long-only, no short selling.
- Uses the same backtest engine and metrics as SMA.
- Best used as a benchmark against active strategies.

### 3) RSI mean reversion

Where implemented:

- Signal generation: internal/strategy/rsi.go
- Portfolio simulation and metrics: internal/backtest/engine.go

Idea:

- Use RSI to buy after weakness and exit after strength.
- Enter long when RSI is below oversold threshold.
- Exit to cash when RSI is above overbought threshold.
- If RSI is between thresholds, keep previous position.

Signal rules:

- Inputs:
  - RSI period: command flag -rsi-period
  - oversold threshold: command flag -rsi-oversold
  - overbought threshold: command flag -rsi-overbought
- Constraints:
  - period must be at least 2
  - oversold must be greater than 0
  - overbought must be less than 100
  - oversold must be less than overbought

Execution model in this app:

- Long-only, no short selling.
- Uses prior-day signal for next-day return.
- Uses Wilder-style smoothing for RSI updates.

Suggested starting parameters:

- period: 14
- oversold: 30
- overbought: 70

Not implemented yet:

- MACD strategy
- Multi-asset rebalancing strategy
- Position sizing/risk parity strategy

## Quick start

```bash
go run ./cmd/backtester
```

Choose strategy explicitly:

```bash
go run ./cmd/backtester -strategy sma
go run ./cmd/backtester -strategy buyhold
go run ./cmd/backtester -strategy rsi
go run ./cmd/backtester -strategy rsi -rsi-period 14 -rsi-oversold 30 -rsi-overbought 70
go run ./cmd/backtester -strategy all
```

Or with Makefile shortcut:

```bash
make run
```

Makefile shortcut for buy-and-hold:

```bash
make run-buyhold
make run-rsi
make run-compare
```

Compare all strategies in one run:

```bash
go run ./cmd/backtester \
  -strategy all \
  -symbols "SPY.US,VGK.US,EWJ.US,GLD.US" \
  -start "2018-01-01"
```

Comparison mode behavior:

- Runs `sma`, `buyhold`, and `rsi` over the same loaded symbol set.
- Prints each strategy's normal per-symbol table.
- Prints a final strategy comparison table with equal-weight averages.
- Prints a recommendation summary for each strategy with per-symbol `BUY/SELL/KEEP` actions.
- Prints a final recommendation (`BUY`, `SELL`, or `KEEP`) for each strategy.
- Uses recommendation timing mode `next-day-safe` by default (uses the prior signal to avoid look-ahead interpretation).
- You can override with `-recommendation-timing close`.
- Supports `-out` CSV export with all strategies in one file (includes a `strategy` column).
- Also writes a second summary CSV (`*_summary.csv`) with one row per strategy, average metrics, and recommendation columns (`buy_count`, `sell_count`, `keep_count`, `final_recommendation`).

Example with custom symbols and date range:

```bash
go run ./cmd/backtester \
  -symbols "SPY.US,VGK.US,EWJ.US,EEM.US,TLT.US,GLD.US" \
  -start "2018-01-01" \
  -short 20 \
  -long 100 \
  -cash 10000 \
  -fee-bps 5 \
  -out report.csv
```

Date defaults:

- If `-end` is not provided, it defaults to the current date.

## Markets catalog with ISIN

Use `markets.csv` to define the exact ETFs/companies you want to explore.

Format:

```csv
name,symbol,isin,enabled
SPDR S&P 500 ETF Trust,SPY.US,US78462F1030,true
```

Notes:

- `symbol`: ticker in Stooq format used for historical prices.
- `isin`: your reference identifier for each market.
- `enabled`: set to `true/false` (or `1/0`, `yes/no`) to include/exclude entries.
- When `-isins` is provided, matching ISIN rows are selected even if `enabled` is `false`.

Run using the catalog:

```bash
go run ./cmd/backtester -use-markets -markets-file markets.csv
```

Run and filter only specific ISINs:

```bash
go run ./cmd/backtester \
  -use-markets \
  -markets-file markets.csv \
  -isins "US78462F1030,US9220428745" \
  -start "2018-01-01"
```

Makefile shortcut for catalog run:

```bash
make run-markets
```

See all shortcuts:

```bash
make
```

## Common global-market tickers (Stooq format)

- US equities: `SPY.US`, `QQQ.US`
- Europe equities: `VGK.US`
- Japan equities: `EWJ.US`
- Emerging markets: `EEM.US`
- Bonds: `TLT.US`
- Gold: `GLD.US`

## Notes

- This is a research tool, not financial advice.
- Prices are daily and strategy execution is simplified for fast iteration.
- For production research, include survivorship-bias controls, realistic slippage, and walk-forward validation.
