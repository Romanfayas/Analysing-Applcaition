package main

import (
	"fmt"
	"log"

	"github.com/halal-equity/backend/internal/providers/marketdata/nsedata"
)

func main() {
	nse := nsedata.NewProvider()
	
	fmt.Println("Testing GetEquityQuote for RELIANCE...")
	quote, err := nse.GetEquityQuote("RELIANCE")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	
	fmt.Printf("Quote: %+v\n", quote)
}
