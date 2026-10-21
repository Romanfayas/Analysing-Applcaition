package nse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

var (
	// Rs / Re regex for dividends: e.g. "Rs 12", "Rs. 12.50", "Re 1", "Re. 1.00", "Rs 11/-"
	reDividendAmount = regexp.MustCompile(`(?i)(?:Rs\.?|Re\.?)\s*([0-9]+(?:\.[0-9]+)?)(?:\s*/-)?`)
	
	// Split regex: e.g. "From Rs 10/- To Re 1/-", "From Rs.10 To Re.2", "From 10 to 1"
	reSplitRatios = regexp.MustCompile(`(?i)(?:from\s+(?:rs\.?|re\.?)?\s*([0-9]+(?:\.[0-9]+)?).*?to\s+(?:rs\.?|re\.?)?\s*([0-9]+(?:\.[0-9]+)?))`)
	
	// Bonus regex: e.g. "Bonus 1:1", "Bonus 1:2", "Bonus Issue 2:1"
	reBonusRatios = regexp.MustCompile(`(?i)bonus.*?([0-9]+)\s*:\s*([0-9]+)`)
	
	// Rights regex: e.g. "Rights 1:4 @ Rs 1257" or "Rights 1:4"
	reRightsRatios = regexp.MustCompile(`(?i)rights.*?([0-9]+)\s*:\s*([0-9]+)(?:\s*@\s*(?:Rs\.?|Re\.?)?\s*([0-9]+(?:\.[0-9]+)?))?`)
)

// ParseNSEDate parses NSE dates formatted as "02-Jan-2006" (e.g. "14-Oct-2026")
func ParseNSEDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	t, err := time.Parse("02-Jan-2006", s)
	if err != nil {
		return nil, fmt.Errorf("failed to parse NSE date %q: %w", s, err)
	}
	utc := t.UTC()
	return &utc, nil
}

// ParseRawCorporateAction normalizes a raw NSE corporate action item into one or more
// distinct, deterministic models.CorporateActionEvent facts.
func ParseRawCorporateAction(
	raw RawNSECorporateAction,
	symbolID int,
	sourceArtifactID *int,
) ([]models.CorporateActionEvent, error) {
	exDate, err := ParseNSEDate(raw.ExDate)
	if err != nil || exDate == nil {
		return nil, fmt.Errorf("invalid exDate %q for symbol %s: %w", raw.ExDate, raw.Symbol, err)
	}

	recDate, _ := ParseNSEDate(raw.RecDate)
	
	subject := strings.TrimSpace(raw.Subject)
	if subject == "" {
		return nil, fmt.Errorf("empty subject for symbol %s on exDate %s", raw.Symbol, raw.ExDate)
	}

	rawJSON, _ := json.Marshal(raw)
	var rawMap map[string]interface{}
	json.Unmarshal(rawJSON, &rawMap)

	// Check if subject is composite (e.g. contains multiple dividend declarations separated by "/")
	// Example: "Interim Dividend Rs 11 Per Share/ Special Dividend Rs 46 Per Share"
	subClauses := splitCompositeSubject(subject)

	var events []models.CorporateActionEvent
	for idx, clause := range subClauses {
		eventIndex := idx + 1
		ev := models.CorporateActionEvent{
			SymbolID:         symbolID,
			Symbol:           raw.Symbol,
			SourceArtifactID: sourceArtifactID,
			SourceSubject:    clause,
			ExDate:           *exDate,
			RecordDate:       recDate,
			EffectiveDate:    exDate,
			Currency:         "INR",
			RawPayload:       rawMap,
		}

		classifyAndExtractFacts(&ev, clause, raw, eventIndex)
		events = append(events, ev)
	}

	return events, nil
}

func splitCompositeSubject(subject string) []string {
	if !strings.Contains(subject, "/") {
		return []string{subject}
	}
	parts := strings.Split(subject, "/")
	var combined []string
	current := ""
	keywords := []string{"special dividend", "final dividend", "interim dividend", "bonus"}

	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "-" {
			// Part of "Rs 10/-" or empty segment
			if current != "" {
				current += "/" + part
			}
			continue
		}

		lower := strings.ToLower(trimmed)
		isDistinctAction := false
		for _, kw := range keywords {
			if strings.HasPrefix(lower, kw) || strings.Contains(lower, kw) {
				isDistinctAction = true
				break
			}
		}

		if i == 0 {
			current = trimmed
		} else if isDistinctAction && current != "" {
			combined = append(combined, current)
			current = trimmed
		} else {
			current += "/" + trimmed
		}
	}
	if current != "" {
		combined = append(combined, current)
	}

	if len(combined) > 1 {
		return combined
	}
	return []string{subject}
}

func classifyAndExtractFacts(ev *models.CorporateActionEvent, clause string, raw RawNSECorporateAction, index int) {
	lower := strings.ToLower(clause)

	exDateStr := ev.ExDate.Format("2006-01-02")
	details := map[string]interface{}{
		"source":      "NSE_INDIA",
		"faceVal":     raw.FaceVal,
		"isin":        raw.ISIN,
		"series":      raw.Series,
		"company":     raw.Comp,
		"clause_text": clause,
	}

	if strings.Contains(lower, "dividend") {
		ev.ActionType = "CASH_DIVIDEND"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:CASH_DIVIDEND:%d", ev.Symbol, exDateStr, index)

		// Extract dividend amount
		matches := reDividendAmount.FindStringSubmatch(clause)
		if len(matches) > 1 {
			amt, err := strconv.ParseFloat(matches[1], 64)
			if err == nil && amt > 0 {
				ev.DividendAmount = &amt
				ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
				ev.CoverageStatus = "PARSED_EVENT"
				details["dividend_per_share"] = amt
			} else {
				ev.VerificationStatus = "AMBIGUOUS"
				ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
				details["parse_error"] = "invalid dividend amount numeric format"
			}
		} else {
			ev.VerificationStatus = "AMBIGUOUS"
			ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
			details["parse_error"] = "dividend amount not found in subject"
		}
	} else if strings.Contains(lower, "split") || strings.Contains(lower, "sub-division") {
		ev.ActionType = "STOCK_SPLIT"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:STOCK_SPLIT:%d", ev.Symbol, exDateStr, index)

		matches := reSplitRatios.FindStringSubmatch(clause)
		if len(matches) > 2 {
			oldRatio, err1 := strconv.ParseFloat(matches[1], 64)
			newRatio, err2 := strconv.ParseFloat(matches[2], 64)
			if err1 == nil && err2 == nil && oldRatio > 0 && newRatio > 0 {
				ev.SplitRatioOld = &oldRatio
				ev.SplitRatioNew = &newRatio
				ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
				ev.CoverageStatus = "PARSED_EVENT"
				details["split_ratio_old"] = oldRatio
				details["split_ratio_new"] = newRatio
			} else {
				ev.VerificationStatus = "AMBIGUOUS"
				ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
			}
		} else {
			ev.VerificationStatus = "AMBIGUOUS"
			ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
		}
	} else if strings.Contains(lower, "bonus") {
		ev.ActionType = "BONUS_ISSUE"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:BONUS_ISSUE:%d", ev.Symbol, exDateStr, index)

		matches := reBonusRatios.FindStringSubmatch(clause)
		if len(matches) > 2 {
			bonus, err1 := strconv.ParseFloat(matches[1], 64)
			held, err2 := strconv.ParseFloat(matches[2], 64)
			if err1 == nil && err2 == nil && bonus > 0 && held > 0 {
				ev.BonusRatioBonus = &bonus
				ev.BonusRatioHeld = &held
				ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
				ev.CoverageStatus = "PARSED_EVENT"
				details["bonus_ratio_bonus"] = bonus
				details["bonus_ratio_held"] = held
			} else {
				ev.VerificationStatus = "AMBIGUOUS"
				ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
			}
		} else {
			ev.VerificationStatus = "AMBIGUOUS"
			ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
		}
	} else if strings.Contains(lower, "rights") {
		ev.ActionType = "RIGHTS_ISSUE"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:RIGHTS_ISSUE:%d", ev.Symbol, exDateStr, index)

		matches := reRightsRatios.FindStringSubmatch(clause)
		if len(matches) > 2 {
			newR, err1 := strconv.ParseFloat(matches[1], 64)
			oldR, err2 := strconv.ParseFloat(matches[2], 64)
			if err1 == nil && err2 == nil && newR > 0 && oldR > 0 {
				ev.RightsRatioNew = &newR
				ev.RightsRatioOld = &oldR
				if len(matches) > 3 && matches[3] != "" {
					subPrice, err3 := strconv.ParseFloat(matches[3], 64)
					if err3 == nil {
						ev.RightsSubscriptionPrice = &subPrice
						details["subscription_price"] = subPrice
					}
				}
				ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
				ev.CoverageStatus = "PARSED_EVENT"
				details["rights_ratio_new"] = newR
				details["rights_ratio_old"] = oldR
			} else {
				ev.VerificationStatus = "AMBIGUOUS"
				ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
			}
		} else {
			ev.VerificationStatus = "AMBIGUOUS"
			ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
		}
	} else if strings.Contains(lower, "demerger") || strings.Contains(lower, "scheme") || strings.Contains(lower, "arrangement") {
		ev.ActionType = "DEMERGER"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:DEMERGER:%d", ev.Symbol, exDateStr, index)
		ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
		ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
		details["note"] = "Demerger scheme declared. Requires statutory apportionment circular for tax cost allocation."
	} else if strings.Contains(lower, "symbol change") || strings.Contains(lower, "name change") {
		ev.ActionType = "SYMBOL_CHANGE"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:SYMBOL_CHANGE:%d", ev.Symbol, exDateStr, index)
		ev.VerificationStatus = "VERIFIED_EXCHANGE_PRIMARY"
		ev.CoverageStatus = "PARSED_EVENT"
		details["note"] = "Instrument lineage mapping."
	} else {
		ev.ActionType = "OTHER"
		ev.EventID = fmt.Sprintf("NSE:%s:%s:OTHER:%d", ev.Symbol, exDateStr, index)
		ev.VerificationStatus = "AMBIGUOUS"
		ev.CoverageStatus = "SECONDARY_EVIDENCE_REQUIRED"
		details["unclassified_subject"] = clause
	}

	ev.Details = details
}
