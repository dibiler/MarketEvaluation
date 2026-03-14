# Global Market Strategy Backtester (Go)

A CLI project for retrieving global market historical data and testing investing strategies.

## What it does

- Downloads daily historical candles from Stooq (free global market source).
- Runs a long-only moving average crossover strategy.
- Computes core backtest metrics:
  - Total return
  - CAGR
  - Max drawdown
  - Sharpe ratio
  - Trade count
- Supports running multiple market symbols in one command.

## Quick start

```bash
go run ./cmd/backtester
```

Example with custom symbols and date range:

```bash
go run ./cmd/backtester \
  -symbols "SPY.US,VGK.US,EWJ.US,EEM.US,TLT.US,GLD.US" \
  -start "2018-01-01" \
  -end "2026-03-14" \
  -short 20 \
  -long 100 \
  -cash 10000 \
  -fee-bps 5 \
  -out report.csv
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
