package database_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getAppTestDB creates a database connection using the non-superuser application role halal_app_user.
func getAppTestDB(t *testing.T) *database.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "192.168.18.43"
	}
	connStr := "postgres://halal_app_user:app_password@" + host + ":5432/halal_equity?sslmode=disable"
	db, err := database.NewDB(connStr)
	require.NoError(t, err)

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Skipping app role test; database not accessible as halal_app_user: %v", err)
	}

	return db
}

// ======================================================================================
// P1.9.4 ADVERSARIAL PRIVILEGE AUDIT SUITE (Tests 1 through 10)
// ======================================================================================

// TestPrivilege_1_ForgeTrustedSourceLabelsAsAppRole verifies that the application role
// supplying forged trusted source labels ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
// with fabricated adjusted closes has those values strictly neutralized to NULL or vendor price.
func TestPrivilege_1_ForgeTrustedSourceLabelsAsAppRole(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	// 1. Quarantined bar (HCLTECH 2026-10-06)
	var hclID int
	err := appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	tsHCL := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)
	fakeAdj1 := 1555.55
	_, err = appDB.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      hclID,
			Timestamp:     tsHCL,
			Open:          1202.0,
			High:          1208.9,
			Low:           1188.2,
			Close:         1204.4,
			Volume:        2500000,
			AdjClose:      &fakeAdj1,
			Source:        "NSE_CORPORATE_ACTION",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterAdjHCL *float64
	var afterSourceHCL string
	err = appDB.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&afterAdjHCL, &afterSourceHCL)
	require.NoError(t, err)
	assert.Nil(t, afterAdjHCL, "Quarantined NULL must remain NULL when app role supplies forged source")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", afterSourceHCL, "Forged source must be rejected")

	// 2. Unadjusted bar (INFY 2026-10-06)
	var infyID int
	err = appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'INFY';").Scan(&infyID)
	require.NoError(t, err)

	tsINFY := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)
	var initOpen, initHigh, initLow, initClose, initAdj float64
	var initVol int64
	err = appDB.QueryRowContext(ctx, "SELECT open, high, low, close, volume, adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", infyID, tsINFY).Scan(
		&initOpen, &initHigh, &initLow, &initClose, &initVol, &initAdj,
	)
	require.NoError(t, err)

	fakeAdjINFY := 2999.99
	_, err = appDB.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      infyID,
			Timestamp:     tsINFY,
			Open:          initOpen,
			High:          initHigh,
			Low:           initLow,
			Close:         initClose,
			Volume:        initVol,
			AdjClose:      &fakeAdjINFY,
			Source:        "PRIMARY_EXCHANGE",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterCloseINFY, afterAdjINFY float64
	var afterSourceINFY string
	err = appDB.QueryRowContext(ctx, "SELECT close, adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", infyID, tsINFY).Scan(&afterCloseINFY, &afterAdjINFY, &afterSourceINFY)
	require.NoError(t, err)
	assert.Equal(t, afterCloseINFY, afterAdjINFY, "Unadjusted bar must retain adjusted_close == close")
	assert.NotEqual(t, "PRIMARY_EXCHANGE", afterSourceINFY, "Forged source must be rejected")
}

// TestPrivilege_2_InsertOrModifyEventToClaimVerifiedAsAppRole verifies that
// the application role cannot insert/modify corporate_action_events, and even
// an admin connection cannot claim VERIFIED_EXCHANGE_PRIMARY without an authentic source artifact.
func TestPrivilege_2_InsertOrModifyEventToClaimVerifiedAsAppRole(t *testing.T) {
	appDB := getAppTestDB(t)
	adminDB := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := adminDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	// 1. App role direct insert is blocked by table permissions
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			'APP:FORGED:EVENT:1', $1, 'HCLTECH', 'CASH_DIVIDEND', 'Forged by App Role',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, hclID)
	assert.Error(t, err, "Application role must be blocked from inserting corporate_action_events")
	assert.True(t, strings.Contains(err.Error(), "permission denied for table corporate_action_events") || strings.Contains(err.Error(), "Event verification rejected"), "Must be rejected by permissions or verification trigger: %v", err)

	// 2. Admin connection attempting to claim VERIFIED_EXCHANGE_PRIMARY without source_artifact_id is blocked by trigger
	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			'ADMIN:FORGED:EVENT:1', $1, 'HCLTECH', 'CASH_DIVIDEND', 'Forged Without Artifact',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, hclID)
	assert.Error(t, err, "Event claiming VERIFIED_EXCHANGE_PRIMARY without artifact must be blocked")
	assert.Contains(t, err.Error(), "requires source_artifact_id to be populated")

	// 3. Admin connection with nonexistent artifact ID is blocked
	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status, source_artifact_id
		) VALUES (
			'ADMIN:FORGED:EVENT:2', $1, 'HCLTECH', 'CASH_DIVIDEND', 'Forged With Fake Artifact',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY', 99999
		);
	`, hclID)
	assert.Error(t, err, "Event with nonexistent artifact ID must be rejected")
	assert.Contains(t, err.Error(), "does not exist in corporate_action_source_artifacts")
}

// TestPrivilege_3_InsertFabricatedAdjustmentFactorAsAppRole verifies that
// neither the application role nor an admin role can insert unverified factors claiming is_verified=true.
func TestPrivilege_3_InsertFabricatedAdjustmentFactorAsAppRole(t *testing.T) {
	appDB := getAppTestDB(t)
	adminDB := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := adminDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	// 1. App role direct insert is blocked by table permissions or ingest guard trigger
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, ex_date, effective_date, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, is_verified, verification_status, factor_category,
			price_factor_applicable, dispute_status, event_id
		) VALUES (
			$1, 'CASH_DIVIDEND', '2026-10-14', '2026-10-14', 0.90,
			1.0, 0.90, 1.0,
			'App Role Insert Attempt', true, 'VERIFIED_EXCHANGE_PRIMARY', 'CASH_DISTRIBUTION',
			true, 'UNDISPUTED', 'TEST:EVENT:APP:1'
		);
	`, hclID)
	assert.Error(t, err, "App role must not be able to insert unverified corporate_action_adjustment_factors")
	assert.True(t, strings.Contains(err.Error(), "permission denied for table corporate_action_adjustment_factors") || strings.Contains(err.Error(), "Ingestion rejected"), "Must be rejected by permissions or ca_factor_ingest_guard: %v", err)


	// 2. Admin attempting to insert factor referencing nonexistent event is blocked by ca_factor_ingest_guard
	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, ex_date, effective_date, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, is_verified, verification_status, factor_category,
			price_factor_applicable, dispute_status, event_id
		) VALUES (
			$1, 'CASH_DIVIDEND', '2026-10-14', '2026-10-14', 0.90,
			1.0, 0.90, 1.0,
			'Admin Insert Attempt', true, 'VERIFIED_EXCHANGE_PRIMARY', 'CASH_DISTRIBUTION',
			true, 'UNDISPUTED', 'NONEXISTENT:EVENT:ID'
		);
	`, hclID)
	assert.Error(t, err, "Factor referencing nonexistent event must be rejected")
	assert.Contains(t, err.Error(), "does not exist in corporate_action_events")
}

// TestPrivilege_4_AttemptToMutateFactorImmutability verifies that factor rows
// are immutable: UPDATE and DELETE are prohibited by ca_factor_guard.
func TestPrivilege_4_AttemptToMutateFactorImmutability(t *testing.T) {
	adminDB := getTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := adminDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	// Attempt UPDATE on verified TCS factor
	_, err = adminDB.ExecContext(ctx, `
		UPDATE corporate_action_adjustment_factors 
		SET total_return_factor = 0.50000000 
		WHERE symbol_id = $1 AND event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1';
	`, tcsID)
	assert.Error(t, err, "UPDATE must be prohibited by immutability trigger")
	assert.Contains(t, err.Error(), "rows are append-only")

	// Attempt DELETE on verified TCS factor
	_, err = adminDB.ExecContext(ctx, `
		DELETE FROM corporate_action_adjustment_factors 
		WHERE symbol_id = $1 AND event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1';
	`, tcsID)
	assert.Error(t, err, "DELETE must be prohibited by immutability trigger")
	assert.Contains(t, err.Error(), "immutable and may not be deleted")
}

// TestPrivilege_5_DirectSQLUpdatesToQuarantinedAndVerifiedBarsAsAppRole verifies that
// direct SQL UPDATE statements by halal_app_user cannot modify quarantined NULLs or verified bars.
func TestPrivilege_5_DirectSQLUpdatesToQuarantinedAndVerifiedBarsAsAppRole(t *testing.T) {
	appDB := getAppTestDB(t)
	adminDB := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, adminDB, ctx)
	})

	var hclID, tcsID int
	err := appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)
	err = appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	tsHCL := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)
	tsTCS := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)

	// Direct update to quarantined HCLTECH bar
	_, err = appDB.ExecContext(ctx, `
		UPDATE ohlcv_daily 
		SET adjusted_close = 1500.0, source = 'NSE_CORPORATE_ACTION' 
		WHERE symbol_id = $1 AND timestamp = $2;
	`, hclID, tsHCL)
	require.NoError(t, err)

	var afterAdjHCL *float64
	var afterSourceHCL string
	err = appDB.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&afterAdjHCL, &afterSourceHCL)
	require.NoError(t, err)
	assert.Nil(t, afterAdjHCL, "Direct UPDATE as app role must NOT un-quarantine NULL")
	assert.Equal(t, "yahoo_finance", afterSourceHCL, "Direct UPDATE must NOT forge source")

	// Direct update to verified TCS bar
	_, err = appDB.ExecContext(ctx, `
		UPDATE ohlcv_daily 
		SET adjusted_close = 2114.3999, source = 'yahoo_finance' 
		WHERE symbol_id = $1 AND timestamp = $2;
	`, tcsID, tsTCS)
	require.NoError(t, err)

	var afterAdjTCS float64
	var afterSourceTCS string
	err = appDB.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, tsTCS).Scan(&afterAdjTCS, &afterSourceTCS)
	require.NoError(t, err)
	assert.InDelta(t, 2103.2190, afterAdjTCS, 0.0001, "Direct UPDATE as app role must NOT overwrite verified adjusted_close")
	assert.Equal(t, "NSE_CORPORATE_ACTION", afterSourceTCS, "Direct UPDATE as app role must NOT downgrade source")
}

// TestPrivilege_6_AppRoleCannotDisableTriggersOrAlterSchema verifies that
// halal_app_user cannot disable triggers, alter relations, or modify schema.
func TestPrivilege_6_AppRoleCannotDisableTriggersOrAlterSchema(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	// 1. Attempt to disable triggers on ohlcv_daily
	_, err := appDB.ExecContext(ctx, "ALTER TABLE ohlcv_daily DISABLE TRIGGER ALL;")
	assert.Error(t, err, "App role must not be able to disable triggers")
	assert.Contains(t, err.Error(), "must be owner of table ohlcv_daily")

	// 2. Attempt to drop trigger
	_, err = appDB.ExecContext(ctx, "DROP TRIGGER trg_ohlcv_daily_quarantine_guard ON ohlcv_daily;")
	assert.Error(t, err, "App role must not be able to drop trigger")
	assert.Contains(t, err.Error(), "must be owner of relation ohlcv_daily")

	// 3. Attempt to alter trigger function
	_, err = appDB.ExecContext(ctx, "CREATE OR REPLACE FUNCTION fn_ohlcv_daily_quarantine_guard() RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END; $$ LANGUAGE plpgsql;")
	assert.Error(t, err, "App role must not be able to modify trigger function in public schema")
	assert.Contains(t, err.Error(), "permission denied for schema public")
}

// TestPrivilege_7_UnauthorizedUsersCannotExecuteSecurityDefiner verifies that
// users without explicit EXECUTE grant cannot execute ca.apply_verified_corporate_action.
func TestPrivilege_7_UnauthorizedUsersCannotExecuteSecurityDefiner(t *testing.T) {
	adminDB := getTestDB(t)
	ctx := context.Background()

	// Create temporary unauthorized user
	unauthRole := "test_unauthorized_exec_role"
	_, err := adminDB.ExecContext(ctx, `
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'test_unauthorized_exec_role') THEN
				CREATE ROLE test_unauthorized_exec_role WITH LOGIN PASSWORD 'unauth_pass';
			END IF;
		END
		$$;
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(ctx, "DROP ROLE IF EXISTS test_unauthorized_exec_role;")
	})

	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "192.168.18.43"
	}
	connStr := "postgres://" + unauthRole + ":unauth_pass@" + host + ":5432/halal_equity?sslmode=disable"
	unauthDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer unauthDB.Close()

	// Attempt execution
	_, err = unauthDB.ExecContext(ctx, "SELECT ca.apply_verified_corporate_action('TCS', '2026-10-05'::date, '2026-10-14'::date, 0.99471203::numeric, 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1');")
	assert.Error(t, err, "Unauthorized user must NOT be permitted to execute ca.apply_verified_corporate_action")
	assert.Contains(t, err.Error(), "permission denied")
}

// TestPrivilege_8_SchemaPermissionsAndProcedureOwnershipAudit verifies that
// procedure ownership is assigned to a non-login role, public execute is revoked,
// and halal_app_user cannot create objects in public or ca.
func TestPrivilege_8_SchemaPermissionsAndProcedureOwnershipAudit(t *testing.T) {
	adminDB := getTestDB(t)
	appDB := getAppTestDB(t)
	ctx := context.Background()

	// 1. Procedure ownership must be ca_security_admin (non-login role)
	var procOwner string
	var isSecDef bool
	var proconfigStr string
	err := adminDB.QueryRowContext(ctx, `
		SELECT r.rolname, p.prosecdef, COALESCE(array_to_string(p.proconfig, ','), '')
		FROM pg_proc p
		JOIN pg_roles r ON r.oid = p.proowner
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'ca' AND p.proname = 'apply_verified_corporate_action';
	`).Scan(&procOwner, &isSecDef, &proconfigStr)
	require.NoError(t, err)
	assert.Equal(t, "ca_security_admin", procOwner, "Procedure owner must be ca_security_admin")
	assert.True(t, isSecDef, "Procedure must be SECURITY DEFINER")
	assert.Contains(t, proconfigStr, "search_path=public, ca, pg_temp", "Procedure must have fixed search_path")

	// 2. PUBLIC must not have execute privilege on procedure
	var publicCanExecute bool
	err = adminDB.QueryRowContext(ctx, `
		SELECT has_function_privilege('public', 'ca.apply_verified_corporate_action(text, date, date, numeric, text)', 'EXECUTE');
	`).Scan(&publicCanExecute)
	require.NoError(t, err)
	assert.False(t, publicCanExecute, "PUBLIC must NOT have execute grant on procedure")

	// 3. halal_app_user cannot CREATE in public or ca schema
	var canCreatePublic, canCreateCA bool
	err = adminDB.QueryRowContext(ctx, "SELECT has_schema_privilege('halal_app_user', 'public', 'CREATE');").Scan(&canCreatePublic)
	require.NoError(t, err)
	assert.False(t, canCreatePublic, "halal_app_user must NOT have CREATE permission on public schema")

	err = adminDB.QueryRowContext(ctx, "SELECT has_schema_privilege('halal_app_user', 'ca', 'CREATE');").Scan(&canCreateCA)
	require.NoError(t, err)
	assert.False(t, canCreateCA, "halal_app_user must NOT have CREATE permission on ca schema")

	// 4. Verify appDB attempts to create table fail
	_, err = appDB.ExecContext(ctx, "CREATE TABLE public.test_shadow_tbl (id int);")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied for schema public")
}

// TestPrivilege_9_ConcurrencyBetweenVerifiedAdjustmentsAndIngestionAsAppRole verifies that
// concurrent execution of ordinary ingestion using halal_app_user racing with
// legitimate corporate action adjustment maintains consistency and avoids deadlocks.
func TestPrivilege_9_ConcurrencyBetweenVerifiedAdjustmentsAndIngestionAsAppRole(t *testing.T) {
	appDB := getAppTestDB(t)
	adminDB := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, adminDB, ctx)
	})

	var tcsID int
	err := appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	tsTCS := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)
	startDate := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	exDate := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

	const workers = 6
	var wg sync.WaitGroup
	wg.Add(workers + 1)

	// Routine 0: Authorized corporate action updater using appDB
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_, _ = appDB.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.99471203, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1")
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Routines 1..6: Concurrent vendor batch ingestion workers using appDB
	for w := 1; w <= workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				conflictingAdj := 2114.3999
				_, _ = appDB.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
					{
						SymbolID:      tcsID,
						Timestamp:     tsTCS,
						Open:          2114.0,
						High:          2140.8999,
						Low:           2063.6001,
						Close:         2114.3999,
						Volume:        1800000 + int64(id*100),
						AdjClose:      &conflictingAdj,
						Source:        "yahoo_finance",
						QualityStatus: "VALID",
						IntervalMins:  1440,
					},
				})
			}
		}(w)
	}

	wg.Wait()

	// Verify TCS state: verified adjusted close and provenance are preserved
	var finalAdj float64
	var finalSource string
	err = appDB.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, tsTCS).Scan(&finalAdj, &finalSource)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", finalSource)
	assert.InDelta(t, 2103.2190, finalAdj, 0.0005)
}

// TestPrivilege_10_OrdinaryIngestionCannotManufactureRelationalProof verifies that
// an adversary attempting the full exploit chain to un-quarantine a bar by inserting
// fake events and fake factors fails closed at every link of the chain.
func TestPrivilege_10_OrdinaryIngestionCannotManufactureRelationalProof(t *testing.T) {
	appDB := getAppTestDB(t)
	adminDB := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	tsHCL := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)

	// Step 1: App role attempts to insert event claiming VERIFIED_EXCHANGE_PRIMARY -> FAILS (permission denied)
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			'EXPLOIT:EVENT:1', $1, 'HCLTECH', 'CASH_DIVIDEND', 'Manufactured Proof Attempt',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, hclID)
	assert.Error(t, err)

	// Step 2: Even if an unverified event is inserted by admin, factor cannot claim is_verified=true
	unverifiedEventID := "EXPLOIT:UNVERIFIED:EVENT:1"
	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			$1, $2, 'HCLTECH', 'CASH_DIVIDEND', 'Unverified Exploit Event',
			'2026-10-14', 'INR', 'UNVERIFIED', 'PARSED_EVENT'
		);
	`, unverifiedEventID, hclID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(ctx, "DELETE FROM corporate_action_events WHERE event_id = $1;", unverifiedEventID)
	})

	_, err = adminDB.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, ex_date, effective_date, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, is_verified, verification_status, factor_category,
			price_factor_applicable, dispute_status, event_id
		) VALUES (
			$1, 'CASH_DIVIDEND', '2026-10-14', '2026-10-14', 0.90,
			1.0, 0.90, 1.0,
			'Exploit Factor Attempt', true, 'VERIFIED_EXCHANGE_PRIMARY', 'CASH_DISTRIBUTION',
			true, 'UNDISPUTED', $2
		);
	`, hclID, unverifiedEventID)
	assert.Error(t, err, "Factor claiming verification on unverified event must fail closed")
	assert.Contains(t, err.Error(), "has status UNVERIFIED, not VERIFIED_EXCHANGE_PRIMARY")

	// Step 3: Attempting to update ohlcv_daily bar using app role -> Quarantined NULL is preserved
	_, err = appDB.ExecContext(ctx, `
		UPDATE ohlcv_daily 
		SET adjusted_close = 1083.96, source = 'NSE_CORPORATE_ACTION'
		WHERE symbol_id = $1 AND timestamp = $2;
	`, hclID, tsHCL)
	require.NoError(t, err)

	var finalAdj *float64
	var finalSource string
	err = appDB.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&finalAdj, &finalSource)
	require.NoError(t, err)
	assert.Nil(t, finalAdj, "Quarantined NULL must remain NULL; manufactured proof was thwarted")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", finalSource)
}
