package stooq

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"global-backtester/internal/marketdata"
)

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
	RateLimit  time.Duration
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
		BaseURL:    "https://stooq.com/q/d/l/",
		RateLimit:  300 * time.Millisecond,
	}
}

func (c *Client) FetchDaily(ctx context.Context, symbol string, start, end time.Time) (marketdata.Series, error) {
	normalized := strings.ToLower(strings.TrimSpace(symbol))
	if normalized == "" {
		return marketdata.Series{}, fmt.Errorf("symbol is required")
	}
	if end.Before(start) {
		return marketdata.Series{}, fmt.Errorf("end date is before start date")
	}

	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return marketdata.Series{}, err
	}
	q := u.Query()
	q.Set("s", normalized)
	q.Set("i", "d")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return marketdata.Series{}, err
	}
	req.Header.Set("User-Agent", "global-backtester/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return marketdata.Series{}, fmt.Errorf("fetch failed for %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return marketdata.Series{}, fmt.Errorf("stooq returned status %d for %s", resp.StatusCode, symbol)
	}

	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1

	_, err = r.Read() // header
	if err != nil {
		return marketdata.Series{}, fmt.Errorf("failed to read CSV header for %s: %w", symbol, err)
	}

	candles := make([]marketdata.Candle, 0, 1024)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return marketdata.Series{}, fmt.Errorf("failed to read CSV for %s: %w", symbol, err)
		}
		if len(record) < 6 || strings.EqualFold(record[0], "date") {
			continue
		}
		if strings.EqualFold(record[1], "n/d") {
			continue
		}

		date, err := time.Parse("2006-01-02", record[0])
		if err != nil {
			continue
		}
		if date.Before(start) || date.After(end) {
			continue
		}

		open, err := strconv.ParseFloat(record[1], 64)
		if err != nil {
			continue
		}
		high, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			continue
		}
		low, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			continue
		}
		closeP, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			continue
		}
		volume, _ := strconv.ParseInt(record[5], 10, 64)

		candles = append(candles, marketdata.Candle{
			Date:   date,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeP,
			Volume: volume,
		})
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Date.Before(candles[j].Date)
	})

	if len(candles) == 0 {
		return marketdata.Series{}, fmt.Errorf("no candles returned for %s in date range", symbol)
	}

	if c.RateLimit > 0 {
		select {
		case <-ctx.Done():
			return marketdata.Series{}, ctx.Err()
		case <-time.After(c.RateLimit):
		}
	}

	return marketdata.Series{Symbol: strings.ToUpper(symbol), Candles: candles}, nil
}
