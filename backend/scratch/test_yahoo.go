package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/providers/marketdata/yahoofinance"
)

func main() {
	yf := yahoofinance.NewProvider()
	
	fmt.Println("Testing Yahoo for RELIANCE...")
	to := time.Now()
	from := to.AddDate(0, 0, -2) // Last 2 days
	
	data, err := yf.GetDailyCandles(context.Background(), "RELIANCE", "NSE", from, to)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	
	fmt.Printf("Fetched %d candles from Yahoo.\n", len(data.Data))
	for _, c := range data.Data {
		fmt.Printf("Timestamp: %v, Close: %v\n", c.Timestamp, c.Close)
	}
}
