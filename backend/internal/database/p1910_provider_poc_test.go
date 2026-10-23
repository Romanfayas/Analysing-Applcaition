package database_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P1.9.10 Automated Release Gates: Historical Data Provider Proof of Concept
// Invariants enforced:
// 1. Proof-of-concept sample payloads exist and have valid structure for TCS, INFY, RELIANCE, HDFCBANK.
// 2. Sample bars preserve OHLC consistency, ascending chronological ordering, non-negative volume, and valid prices.
// 3. Price-series consumer separation: unadjusted nominal execution prices remain distinct from split/dividend factor calculations.
// 4. Primary corporate action records feature verified authority (NSE_INDIA) and valid artifact SHA-256 digests.
// 5. POC validation report certifies zero gaps, zero duplicates, zero invalid OHLCV relationships, and specifies conditional GO verdict.
// 6. Platform safety invariants preserved: 23,132 quarantined bars untouched, 0 lineage bars, Shariah fail-closed (31 DATA_INSUFFICIENT).

type P1910ValidationOutput struct {
	Version                string   `json:"version"`
	GeneratedAt            string   `json:"generated_at"`
	EvaluationTarget       string   `json:"evaluation_target"`
	TestedSymbols          []string `json:"tested_symbols"`
	SampleCoverageAnalysis struct {
		HistoricalDailyOHLCVAvailable  bool   `json:"historical_daily_ohlcv_available"`
		RequiredDateRangeCovered       string `json:"required_date_range_covered"`
		MissingBarsCount               int    `json:"missing_bars_count"`
		DuplicateBarsCount             int    `json:"duplicate_bars_count"`
		InvalidOHLCVRelationshipsCount int    `json:"invalid_ohlcv_relationships_count"`
		ZeroVolumePhantomBarsCount     int    `json:"zero_volume_phantom_bars_count"`
		TimezoneHandling               string `json:"timezone_handling"`
	} `json:"sample_coverage_analysis"`
	PriceSeriesSeparationAudit struct {
		RawNominalPricesSeparate    bool   `json:"raw_nominal_prices_separate"`
		SplitAdjustedPricesSeparate bool   `json:"split_adjusted_prices_separate"`
		TotalReturnPricesSeparate   bool   `json:"total_return_prices_separate"`
		TaxCostAllocationSeparate   bool   `json:"tax_cost_allocation_separate"`
		Finding                     string `json:"finding"`
	} `json:"price_series_separation_audit"`
	OverallProviderPOCVerdict struct {
		Recommendation     string   `json:"recommendation"`
		RoleInHalalEquity  string   `json:"role_in_halalequity"`
		UnresolvedBlockers []string `json:"unresolved_blockers"`
	} `json:"overall_provider_poc_verdict"`
}

type P1910SamplePayload struct {
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint"`
	Symbol          string `json:"symbol"`
	SampleDateRange struct {
		Start     string `json:"start"`
		End       string `json:"end"`
		TotalBars int    `json:"total_bars"`
	} `json:"sample_date_range"`
	PayloadSHA256   string `json:"payload_sha256"`
	VendorSemantics struct {
		PriceSeriesType               string `json:"price_series_type"`
		DividendAdjustedInFeed        bool   `json:"dividend_adjusted_in_feed"`
		SplitBonusAdjustedInFeed      bool   `json:"split_bonus_adjusted_in_feed"`
		VendorAdjustmentDocumentation string `json:"vendor_adjustment_documentation"`
	} `json:"vendor_semantics"`
	PrimaryExchangeCorporateActions []struct {
		EventID            string  `json:"event_id"`
		ActionType         string  `json:"action_type"`
		ExDate             string  `json:"ex_date"`
		VerificationStatus string  `json:"verification_status"`
		IssuingAuthority   string  `json:"issuing_authority"`
		DocumentIdentifier string  `json:"document_identifier"`
		ArtifactSHA256     string  `json:"artifact_sha256"`
	} `json:"primary_exchange_corporate_actions"`
	SampleBars []struct {
		Timestamp string  `json:"timestamp"`
		Open      float64 `json:"open"`
		High      float64 `json:"high"`
		Low       float64 `json:"low"`
		Close     float64 `json:"close"`
		Volume    int64   `json:"volume"`
	} `json:"sample_bars"`
}

func readP1910File(t *testing.T, relPath string) []byte {
	paths := []string{
		relPath,
		"../../" + relPath,
		"../../../" + relPath,
		"/Users/fayas/Downloads/ICO-Application/analysis application/" + relPath,
	}
	var data []byte
	var err error
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		data, err = os.ReadFile(abs)
		if err == nil {
			return data
		}
	}
	require.NoError(t, err, "Failed to read file: "+relPath)
	return nil
}

// 1. Proof-of-concept sample payloads exist and have valid structure for TCS, INFY, RELIANCE, HDFCBANK
func TestP1910_1_ProviderSampleCoverageAndPayloadIntegrity(t *testing.T) {
	symbols := []string{"tcs", "infy", "reliance", "hdfcbank"}
	for _, sym := range symbols {
		dataBytes := readP1910File(t, "data/poc/truedata_poc_sample_"+sym+".json")
		var sample P1910SamplePayload
		err := json.Unmarshal(dataBytes, &sample)
		require.NoError(t, err, "Failed to unmarshal POC sample for "+sym)

		assert.Equal(t, "TRUDATA_AUTHORISED_VENDOR", sample.Provider)
		assert.Equal(t, strings.ToUpper(sym), sample.Symbol)
		assert.NotEmpty(t, sample.Endpoint)
		assert.GreaterOrEqual(t, sample.SampleDateRange.TotalBars, 500, "Must have at least 500 bars for "+sym)
		assert.Equal(t, len(sample.SampleBars), sample.SampleDateRange.TotalBars)
		assert.Len(t, sample.PayloadSHA256, 64, "Payload SHA256 must be valid 64-char hex")
	}
}

// 2. Sample bars preserve OHLC consistency, ascending chronological ordering, non-negative volume, and valid prices
func TestP1910_2_SampleBarsOHLCVIntegrity(t *testing.T) {
	symbols := []string{"tcs", "infy", "reliance", "hdfcbank"}
	for _, sym := range symbols {
		dataBytes := readP1910File(t, "data/poc/truedata_poc_sample_"+sym+".json")
		var sample P1910SamplePayload
		err := json.Unmarshal(dataBytes, &sample)
		require.NoError(t, err)

		var prevTS string
		for i, bar := range sample.SampleBars {
			if prevTS != "" {
				assert.True(t, bar.Timestamp > prevTS, "%s bar %d timestamp not strictly ascending: %s <= %s", sym, i, bar.Timestamp, prevTS)
			}
			prevTS = bar.Timestamp

			assert.Greater(t, bar.Open, 0.0, "%s bar %d open must be positive", sym, i)
			assert.Greater(t, bar.High, 0.0, "%s bar %d high must be positive", sym, i)
			assert.Greater(t, bar.Low, 0.0, "%s bar %d low must be positive", sym, i)
			assert.Greater(t, bar.Close, 0.0, "%s bar %d close must be positive", sym, i)
			assert.GreaterOrEqual(t, bar.Volume, int64(0), "%s bar %d volume must be non-negative", sym, i)

			assert.GreaterOrEqual(t, bar.High, bar.Low, "%s bar %d high < low", sym, i)
			assert.GreaterOrEqual(t, bar.High, bar.Open, "%s bar %d high < open", sym, i)
			assert.GreaterOrEqual(t, bar.High, bar.Close, "%s bar %d high < close", sym, i)
			assert.LessOrEqual(t, bar.Low, bar.Open, "%s bar %d low > open", sym, i)
			assert.LessOrEqual(t, bar.Low, bar.Close, "%s bar %d low > close", sym, i)
		}
	}
}

// 3. Price-series consumer separation: unadjusted nominal execution prices remain distinct
func TestP1910_3_PriceSeriesConsumerSeparation(t *testing.T) {
	symbols := []string{"tcs", "infy", "reliance", "hdfcbank"}
	for _, sym := range symbols {
		dataBytes := readP1910File(t, "data/poc/truedata_poc_sample_"+sym+".json")
		var sample P1910SamplePayload
		err := json.Unmarshal(dataBytes, &sample)
		require.NoError(t, err)

		sem := sample.VendorSemantics
		assert.Equal(t, "NOMINAL_RAW_EXECUTION_PRICES", sem.PriceSeriesType)
		assert.False(t, sem.DividendAdjustedInFeed, "Vendor feed must NOT adjust for cash dividends")
		assert.False(t, sem.SplitBonusAdjustedInFeed, "Vendor feed must NOT conflate splits into raw nominal prices")
		assert.NotEmpty(t, sem.VendorAdjustmentDocumentation)
	}
}

// 4. Primary corporate action records feature verified authority (NSE_INDIA) and valid artifact SHA-256 digests
func TestP1910_4_CorporateActionProvenanceAndPrimaryCrossValidation(t *testing.T) {
	dataBytes := readP1910File(t, "data/poc/truedata_poc_sample_tcs.json")
	var sample P1910SamplePayload
	err := json.Unmarshal(dataBytes, &sample)
	require.NoError(t, err)

	require.NotEmpty(t, sample.PrimaryExchangeCorporateActions, "TCS must have primary corporate actions")
	var verifiedCount int
	for _, ca := range sample.PrimaryExchangeCorporateActions {
		if ca.VerificationStatus == "VERIFIED_EXCHANGE_PRIMARY" {
			verifiedCount++
			assert.Equal(t, "NSE_INDIA", ca.IssuingAuthority)
			assert.NotEmpty(t, ca.DocumentIdentifier)
			assert.Len(t, ca.ArtifactSHA256, 64)
		}
	}
	assert.GreaterOrEqual(t, verifiedCount, 1, "Must have at least 1 verified primary corporate action record")
}

// 5. POC validation report certifies zero gaps, zero duplicates, zero invalid OHLCV relationships, and specifies conditional GO verdict
func TestP1910_5_POCValidationReportAndGONoGoRecommendation(t *testing.T) {
	dataBytes := readP1910File(t, "data/poc/poc_validation_output.json")
	var valOut P1910ValidationOutput
	err := json.Unmarshal(dataBytes, &valOut)
	require.NoError(t, err)

	assert.True(t, valOut.SampleCoverageAnalysis.HistoricalDailyOHLCVAvailable)
	assert.Equal(t, 0, valOut.SampleCoverageAnalysis.MissingBarsCount)
	assert.Equal(t, 0, valOut.SampleCoverageAnalysis.DuplicateBarsCount)
	assert.Equal(t, 0, valOut.SampleCoverageAnalysis.InvalidOHLCVRelationshipsCount)
	assert.Equal(t, 0, valOut.SampleCoverageAnalysis.ZeroVolumePhantomBarsCount)

	assert.True(t, valOut.PriceSeriesSeparationAudit.RawNominalPricesSeparate)
	assert.True(t, valOut.PriceSeriesSeparationAudit.SplitAdjustedPricesSeparate)
	assert.True(t, valOut.PriceSeriesSeparationAudit.TotalReturnPricesSeparate)
	assert.True(t, valOut.PriceSeriesSeparationAudit.TaxCostAllocationSeparate)

	assert.Equal(t, "GO_FOR_RAW_OHLCV_INGESTION_CONDITIONAL_ON_SUBSCRIPTION_APPROVAL", valOut.OverallProviderPOCVerdict.Recommendation)
	assert.GreaterOrEqual(t, len(valOut.OverallProviderPOCVerdict.UnresolvedBlockers), 3)
}

// 6. Platform safety invariants preserved: 23,132 quarantined bars untouched, 0 lineage bars, Shariah fail-closed
func TestP1910_6_PlatformSafetyInvariantsPreserved(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 23,132 quarantined bars in ohlcv_daily
	var quarantinedCount int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM ohlcv_daily WHERE adjusted_close IS NULL;`).Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Quarantined bars count must remain exactly 23,132")

	// Invariant: 0 bars for restructuring/lineage instruments
	var lineageBarCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
	`).Scan(&lineageBarCount)
	require.NoError(t, err)
	assert.Equal(t, 0, lineageBarCount, "Restructuring/lineage instruments must have 0 bars")

	// Invariant: Shariah screening fail-closed (0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT)
	var passCount, failCount, dataInsufCount int
	err = db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (s.symbol) s.symbol, sc.status
			FROM symbols s
			JOIN shariah_screenings sc ON s.id = sc.symbol_id
			WHERE s.is_active = true
			ORDER BY s.symbol, sc.screening_date DESC
		)
		SELECT 
			count(*) FILTER (WHERE status = 'PASS'),
			count(*) FILTER (WHERE status = 'FAIL'),
			count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT')
		FROM latest_screenings;
	`).Scan(&passCount, &failCount, &dataInsufCount)
	require.NoError(t, err)

	assert.Equal(t, 0, passCount, "Zero active equities may be marked PASS without audited interest income footnotes")
	assert.Equal(t, 20, failCount, "Exactly 20 active equities are verified FAIL")
	assert.Equal(t, 31, dataInsufCount, "Exactly 31 active equities are DATA_INSUFFICIENT")
}
