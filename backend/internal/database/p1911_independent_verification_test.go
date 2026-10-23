package database_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P1.9.11 Automated Release Gates: Independent Provider Data Verification and Research Readiness Audit
// Enforces:
// 1. Synthetic or fixture data misrepresentation protection: POC samples explicitly labeled as transformed fixtures.
// 2. Corporate-action artifact foreign key integrity and SHA-256 checksum binding.
// 3. Controlled archive provenance limitation: 27 core equities missing primary exchange circulars.
// 4. Quarantine bypass prevention: strictly 23,132 quarantined bars with trigger protection.
// 5. Price series consumer separation: raw nominal prices isolated from factor adjustments, tax cost non-price.
// 6. Unresolved instrument lineage protection: 5 lineage entities with exactly 0 daily bars.
// 7. Shariah fail-closed screening: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT (null interest income ratio).
// 8. Canonical universe reconciliation: 52 catalog, 51 active, 47 core, 23,782 total bars.

type P1911ValidationReport struct {
	Version              string `json:"version"`
	DataProvenanceVerdict struct {
		SampleOrigin                      string `json:"sample_origin"`
		LiveAPIStreamAuthenticated        bool   `json:"live_api_stream_authenticated"`
		LiveProviderStatus                string `json:"live_provider_status"`
		SyntheticOrFixtureDataDisclaimer string `json:"synthetic_or_fixture_data_disclaimer"`
	} `json:"data_provenance_verdict"`
	OverallProviderPOCVerdict struct {
		Recommendation     string   `json:"recommendation"`
		RoleInHalalEquity  string   `json:"role_in_halalequity"`
		UnresolvedBlockers []string `json:"unresolved_blockers"`
	} `json:"overall_provider_poc_verdict"`
}

type P1911SamplePayloadDetail struct {
	Provider           string `json:"provider"`
	Symbol             string `json:"symbol"`
	DataProvenanceType string `json:"data_provenance_type"`
	IsLiveDownload     bool   `json:"is_live_download"`
	AcquisitionPath    string `json:"acquisition_path"`
	VendorSemantics    struct {
		PriceSeriesType          string `json:"price_series_type"`
		DividendAdjustedInFeed   bool   `json:"dividend_adjusted_in_feed"`
		SplitBonusAdjustedInFeed bool   `json:"split_bonus_adjusted_in_feed"`
	} `json:"vendor_semantics"`
}

func readP1911File(t *testing.T, relPath string) []byte {
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

// 1. Synthetic or fixture data misrepresentation protection
func TestP1911_1_SyntheticOrFixtureDataMisrepresentationProtection(t *testing.T) {
	dataBytes := readP1911File(t, "data/poc/poc_validation_output.json")
	var report P1911ValidationReport
	err := json.Unmarshal(dataBytes, &report)
	require.NoError(t, err)

	prov := report.DataProvenanceVerdict
	assert.Equal(t, "TRANSFORMED_SCHEMA_FIXTURE_FROM_LOCAL_POSTGRESQL", prov.SampleOrigin)
	assert.False(t, prov.LiveAPIStreamAuthenticated, "Live API stream must not be falsely marked authenticated")
	assert.Equal(t, "DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION", prov.LiveProviderStatus)
	assert.Contains(t, prov.SyntheticOrFixtureDataDisclaimer, "fixtures do NOT qualify as live provider validation evidence")

	symbols := []string{"tcs", "infy", "reliance", "hdfcbank"}
	for _, sym := range symbols {
		sBytes := readP1911File(t, "data/poc/truedata_poc_sample_"+sym+".json")
		var sPayload P1911SamplePayloadDetail
		err = json.Unmarshal(sBytes, &sPayload)
		require.NoError(t, err)

		assert.Equal(t, "TRANSFORMED_SCHEMA_FIXTURE", sPayload.DataProvenanceType)
		assert.False(t, sPayload.IsLiveDownload)
		assert.Contains(t, sPayload.AcquisitionPath, "Local PostgreSQL database query")
	}
}

// 2. Corporate-action artifact foreign key integrity and SHA-256 checksum binding
func TestP1911_2_CorporateActionArtifactIntegrityAndFKProtection(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. All 64 events link to an existing artifact
	var orphanedEvents int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM corporate_action_events ev
		LEFT JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		WHERE art.id IS NULL;
	`).Scan(&orphanedEvents)
	require.NoError(t, err)
	assert.Equal(t, 0, orphanedEvents, "Zero orphaned events permitted")

	// 2. All 23 source artifacts have valid SHA-256
	var validArtifacts int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM corporate_action_source_artifacts
		WHERE length(sha256_checksum) = 64;
	`).Scan(&validArtifacts)
	require.NoError(t, err)
	assert.Equal(t, 23, validArtifacts, "All 23 source artifacts must have 64-char hex checksums")
}

// 3. Controlled archive provenance limitation: 27 core equities missing primary exchange circulars
func TestP1911_3_SourceProvenanceControlledArchiveLimitations(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Stored artifacts are local snapshot archives
	var controlledArchiveCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM corporate_action_source_artifacts
		WHERE source_url LIKE 'CONTROLLED_ARCHIVE:%';
	`).Scan(&controlledArchiveCount)
	require.NoError(t, err)
	assert.Equal(t, 23, controlledArchiveCount, "All 23 source artifacts must be marked CONTROLLED_ARCHIVE")

	// Invariant: Exactly 27 active core equities have zero primary evidence
	var unbackedCoreCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM symbols s
		WHERE s.is_active = true
		  AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
		  AND s.id NOT IN (SELECT DISTINCT symbol_id FROM corporate_action_events);
	`).Scan(&unbackedCoreCount)
	require.NoError(t, err)
	assert.Equal(t, 27, unbackedCoreCount, "27 active core equities missing primary evidence blocks research")
}

// 4. Quarantine bypass prevention: strictly 23,132 quarantined bars with trigger protection
func TestP1911_4_QuarantineBypassPreventionAndInvariantPreservation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: exactly 23,132 quarantined bars
	var quarantinedCount int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM ohlcv_daily WHERE adjusted_close IS NULL;`).Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Quarantined bars invariant violated")

	// Invariant: trigger trg_ohlcv_daily_quarantine_guard exists
	var triggerCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM information_schema.triggers
		WHERE event_object_table = 'ohlcv_daily'
		  AND trigger_name = 'trg_ohlcv_daily_quarantine_guard';
	`).Scan(&triggerCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, triggerCount, 1, "Quarantine guard trigger must protect ohlcv_daily")
}

// 5. Price series consumer separation: raw nominal prices isolated from factor adjustments
func TestP1911_5_NominalVsTotalReturnConsumerSeparation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Tax cost allocation is never a price adjustment factor
	var invalidTaxFactors int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_tax_cost_allocations
		WHERE price_factor_applicable = true;
	`).Scan(&invalidTaxFactors)
	require.NoError(t, err)
	assert.Equal(t, 0, invalidTaxFactors, "Tax-cost allocation must never be used as price factor")
}

// 6. Unresolved instrument lineage protection: 5 lineage entities with exactly 0 daily bars
func TestP1911_6_UnresolvedInstrumentLineageProtection(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var lineageBars int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
	`).Scan(&lineageBars)
	require.NoError(t, err)
	assert.Equal(t, 0, lineageBars, "Lineage restructuring entities must have exactly 0 bars")
}

// 7. Shariah fail-closed screening: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT
func TestP1911_7_MissingStatutoryShariahEvidenceFailClosed(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var passCount, failCount, dataInsufCount, fakePassCount int
	err := db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (s.symbol) s.symbol, sc.status, sc.interest_income_ratio
			FROM symbols s
			JOIN shariah_screenings sc ON s.id = sc.symbol_id
			WHERE s.is_active = true
			ORDER BY s.symbol, sc.screening_date DESC
		)
		SELECT 
			count(*) FILTER (WHERE status = 'PASS'),
			count(*) FILTER (WHERE status = 'FAIL'),
			count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT'),
			count(*) FILTER (WHERE status = 'PASS' AND interest_income_ratio IS NULL)
		FROM latest_screenings;
	`).Scan(&passCount, &failCount, &dataInsufCount, &fakePassCount)
	require.NoError(t, err)

	assert.Equal(t, 0, passCount, "Zero equities may be marked PASS without audited interest income footnotes")
	assert.Equal(t, 20, failCount, "Exactly 20 verified FAIL equities")
	assert.Equal(t, 31, dataInsufCount, "Exactly 31 DATA_INSUFFICIENT equities")
	assert.Equal(t, 0, fakePassCount, "No equity with NULL interest_income_ratio may be marked PASS")
}

// 8. Canonical universe reconciliation: 52 catalog, 51 active, 47 core, 23,782 total bars
func TestP1911_8_CanonicalUniverseReconciliation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var totalSyms, activeSyms, archSyms int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE is_active = true),
			count(*) FILTER (WHERE is_active = false)
		FROM symbols;
	`).Scan(&totalSyms, &activeSyms, &archSyms)
	require.NoError(t, err)
	assert.Equal(t, 52, totalSyms)
	assert.Equal(t, 51, activeSyms)
	assert.Equal(t, 1, archSyms)

	var totalBars, quarantinedBars, populatedBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE adjusted_close IS NULL),
			count(*) FILTER (WHERE adjusted_close IS NOT NULL)
		FROM ohlcv_daily;
	`).Scan(&totalBars, &quarantinedBars, &populatedBars)
	require.NoError(t, err)
	assert.Equal(t, 23782, totalBars, "Total daily bars must be exactly 23,782")
	assert.Equal(t, 23132, quarantinedBars, "Quarantined bars must be exactly 23,132")
	assert.Equal(t, 650, populatedBars, "Populated bars must be exactly 650")
}
