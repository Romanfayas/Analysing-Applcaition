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

// NSECalendarVersion identifies the official versioned exchange holiday calendar.
const NSECalendarVersion = "NSE_EQ_CALENDAR_2024-10_to_2026-12_V1"

// FullDayHolidays contains official full-day NSE trading holidays (NSE_EQ_CALENDAR_2024-10_to_2026-12_V1).
var FullDayHolidays = map[string]string{
	"2024-11-15": "Guru Nanak Jayanti",
	"2024-11-20": "Maharashtra Assembly Elections",
	"2024-12-25": "Christmas",
	"2025-02-26": "Mahashivratri",
	"2025-03-14": "Holi",
	"2025-03-31": "Id-Ul-Fitr",
	"2025-04-10": "Shri Mahavir Jayanti",
	"2025-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
	"2025-04-18": "Good Friday",
	"2025-05-01": "Maharashtra Day",
	"2025-08-15": "Independence Day",
	"2025-08-27": "Ganesh Chaturthi",
	"2025-10-02": "Mahatma Gandhi Jayanti",
	"2025-10-22": "Diwali Balipratipada",
	"2025-11-05": "Prakash Gurpurb",
	"2025-12-25": "Christmas",
	"2026-01-15": "Municipal Elections",
	"2026-01-26": "Republic Day",
	"2026-03-03": "Holi",
	"2026-03-26": "Ram Navami",
	"2026-03-31": "Id-Ul-Fitr",
	"2026-04-03": "Good Friday",
	"2026-04-14": "Dr. Ambedkar Jayanti",
	"2026-05-01": "Maharashtra Day",
	"2026-05-28": "Bakri Id",
	"2026-06-26": "Muharram",
	"2026-09-14": "Milad-un-Nabi",
	"2026-10-02": "Mahatma Gandhi Jayanti",
	"2026-10-20": "Dussehra",
	"2026-11-10": "Diwali Laxmi Pujan",
	"2026-11-24": "Guru Nanak Jayanti",
	"2026-12-25": "Christmas",
}

// IsTradingHoliday checks if the given time in IST falls on an exchange trading holiday.
func IsTradingHoliday(t time.Time) (bool, string) {
	tIST := t.In(Location)
	dateKey := tIST.Format("2006-01-02")
	holidayName, exists := FullDayHolidays[dateKey]
	return exists, holidayName
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

	// Exchange Holidays
	if isHoliday, _ := IsTradingHoliday(nowIST); isHoliday {
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

// GetFreshnessStatus determines if the provided market data timestamp is considered fresh based on the current market session.
// Rules:
// 1. Zero or uninitialized timestamps are UNAVAILABLE.
// 2. During OPEN session: data must be from today and within 15 minutes of current time to be FRESH; otherwise STALE.
// 3. During PRE_MARKET session: data should be from the previous trading session (within 72 hours).
// 4. During CLOSED / POST_MARKET session:
//    - If today was a trading weekday and market has already closed for the day, data must be from today's session to be LAST_AVAILABLE.
//      If it is from a previous day when today had an active session, it missed today's session and is STALE.
//    - On weekends (Saturday/Sunday) or before market open on Monday: data from Friday's close (within 72 hours) is LAST_AVAILABLE.
//    - If data is older than 72 hours, it is STALE.
func GetFreshnessStatus(dataTime time.Time, now time.Time) string {
	if dataTime.IsZero() {
		return "UNAVAILABLE"
	}

	loc := Location
	nowIST := now.In(loc)
	dataIST := dataTime.In(loc)

	session := GetCurrentSession(now)

	// If the market is actively open:
	if session == SessionOpen {
		// Must be from today
		if dataIST.Year() != nowIST.Year() || dataIST.Month() != nowIST.Month() || dataIST.Day() != nowIST.Day() {
			return "STALE"
		}
		// Must be within 15 minutes of current time
		if now.Sub(dataTime) > 15*time.Minute {
			return "STALE"
		}
		return "FRESH"
	}

	// If market is pre-market:
	if session == SessionPreMarket {
		if now.Sub(dataTime) <= 72*time.Hour {
			return "LAST_AVAILABLE"
		}
		return "STALE"
	}

	// If market is closed or post-market:
	isHoliday, _ := IsTradingHoliday(nowIST)
	if isHoliday {
		// Market is closed for an official exchange holiday today.
		// Quotes from the most recent preceding trading session (within 96 hours to cover long weekends) are LAST_AVAILABLE.
		if now.Sub(dataTime) <= 96*time.Hour {
			return "LAST_AVAILABLE"
		}
		return "STALE"
	}

	isWeekday := nowIST.Weekday() >= time.Monday && nowIST.Weekday() <= time.Friday
	marketCloseTime := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 15, 30, 0, 0, loc)

	if isWeekday && nowIST.After(marketCloseTime) {
		// Today's trading session already occurred on an active weekday.
		// If the data is from today, it represents the latest available session close.
		if dataIST.Year() == nowIST.Year() && dataIST.Month() == nowIST.Month() && dataIST.Day() == nowIST.Day() {
			return "LAST_AVAILABLE"
		}
		// If it's missing today's entire trading day, it is stale.
		return "STALE"
	}

	// Before today's market opens (early morning weekday) or on weekend:
	if now.Sub(dataTime) <= 72*time.Hour {
		return "LAST_AVAILABLE"
	}

	return "STALE"
}
