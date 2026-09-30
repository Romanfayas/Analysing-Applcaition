package models

import (
	"time"

	"github.com/google/uuid"
)

// ==================================================
// Core Models
// ==================================================

// User represents an authenticated user of the platform.
type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Name         string    `json:"name" db:"name"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// AuditLog records security-relevant actions.
type AuditLog struct {
	ID         int64     `json:"id" db:"id"`
	UserID     *uuid.UUID `json:"user_id,omitempty" db:"user_id"`
	Action     string    `json:"action" db:"action"`
	Resource   string    `json:"resource,omitempty" db:"resource"`
	ResourceID string    `json:"resource_id,omitempty" db:"resource_id"`
	Details    any       `json:"details,omitempty" db:"details"`
	IPAddress  string    `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent  string    `json:"user_agent,omitempty" db:"user_agent"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// ==================================================
// Enums
// ==================================================

type DataQualityStatus string

const (
	DataQualityValid     DataQualityStatus = "VALID"
	DataQualityMissing   DataQualityStatus = "MISSING"
	DataQualityStale     DataQualityStatus = "STALE"
	DataQualityDuplicate DataQualityStatus = "DUPLICATE"
	DataQualitySuspect   DataQualityStatus = "SUSPECT"
	DataQualityCorrected DataQualityStatus = "CORRECTED"
)

type ShariahStatus string

const (
	ShariahPass             ShariahStatus = "PASS"
	ShariahFail             ShariahStatus = "FAIL"
	ShariahReviewRequired   ShariahStatus = "REVIEW_REQUIRED"
	ShariahDataInsufficient ShariahStatus = "DATA_INSUFFICIENT"
)

type SignalType string

const (
	SignalStrongBuy   SignalType = "STRONG_BUY"
	SignalBuy         SignalType = "BUY"
	SignalWatch       SignalType = "WATCH"
	SignalAvoid       SignalType = "AVOID"
	SignalStrongAvoid SignalType = "STRONG_AVOID"
)

type IPODecision string

const (
	IPOApply          IPODecision = "APPLY"
	IPOWatch          IPODecision = "WATCH"
	IPOAvoid          IPODecision = "AVOID"
	IPOReviewRequired IPODecision = "REVIEW_REQUIRED"
)

type OrderSide string

const (
	OrderBuy  OrderSide = "BUY"
	OrderSell OrderSide = "SELL"
)

type FinancialPeriodType string

const (
	PeriodAnnual    FinancialPeriodType = "ANNUAL"
	PeriodQuarterly FinancialPeriodType = "QUARTERLY"
	PeriodHalfYear  FinancialPeriodType = "HALF_YEARLY"
	PeriodTTM       FinancialPeriodType = "TTM"
)
