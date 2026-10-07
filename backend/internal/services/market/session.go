package market

import (
	"log"
	"time"
)

var (
	Location *time.Location
)

func init() {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Printf("Warning: Could not load Asia/Kolkata timezone: %v", err)
		Location = time.UTC
	} else {
		Location = loc
	}
}

// SessionStatus represents the current state of the market.
type SessionStatus string

const (
	SessionOpen      SessionStatus = "OPEN"
	SessionClosed    SessionStatus = "CLOSED"
	SessionPreMarket SessionStatus = "PRE_MARKET"
	SessionPostMarket SessionStatus = "POST_MARKET"
)

// GetCurrentSession returns the current market session status.
func GetCurrentSession(now time.Time) SessionStatus {
	nowIST := now.In(Location)

	// Weekends
	if nowIST.Weekday() == time.Saturday || nowIST.Weekday() == time.Sunday {
		return SessionClosed
	}

	// Calculate boundaries
	preMarketOpen := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 9, 0, 0, 0, Location)
	marketOpen := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 9, 15, 0, 0, Location)
	marketClose := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 15, 30, 0, 0, Location)
	postMarketClose := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 16, 0, 0, 0, Location)

	if nowIST.Before(preMarketOpen) {
		return SessionClosed
	}
	if nowIST.After(preMarketOpen) && nowIST.Before(marketOpen) {
		return SessionPreMarket
	}
	if nowIST.After(marketOpen) && nowIST.Before(marketClose) {
		return SessionOpen
	}
	if nowIST.After(marketClose) && nowIST.Before(postMarketClose) {
		return SessionPostMarket
	}
	return SessionClosed
}

// GetFreshnessStatus determines if the provided data timestamp is considered fresh based on the current market session.
func GetFreshnessStatus(dataTime time.Time, now time.Time) string {
	session := GetCurrentSession(now)
	
	// If the market is open, data should ideally be from today and recent (within 15 mins for paper trading)
	if session == SessionOpen {
		if now.Sub(dataTime) > 15*time.Minute {
			return "STALE"
		}
		return "FRESH"
	}
	
	// If the market is closed, the most recent data should be from the last market close.
	// For simplicity, if it's within the last 72 hours (weekend cover), consider it LAST_AVAILABLE.
	if now.Sub(dataTime) > 72*time.Hour {
		return "STALE"
	}
	
	return "LAST_AVAILABLE"
}
