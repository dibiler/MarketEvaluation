APP=./cmd/backtester

run:
	go run $(APP)

run-buyhold:
	go run $(APP) -strategy buyhold

run-rsi:
	go run $(APP) -strategy rsi -rsi-period 14 -rsi-oversold 30 -rsi-overbought 70

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

run-sample:
	go run $(APP) -symbols "SPY.US,VGK.US,EWJ.US,EEM.US,TLT.US,GLD.US" -start "2018-01-01" -end "2026-03-14" -short 20 -long 100 -cash 10000 -fee-bps 5

run-markets:
	go run $(APP) -use-markets -markets-file markets.csv -start "2018-01-01" -end "2026-03-14" -short 20 -long 100 -cash 10000 -fee-bps 5
