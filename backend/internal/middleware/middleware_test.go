package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/halal-equity/backend/internal/auth"
)

func call(t *testing.T, h http.Handler, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestDeletedOrDisabledAccountsGet401AndLookupsAreCached(t *testing.T) {
	jm := auth.NewJWTManager("test-secret-test-secret-test-secret", time.Hour)
	alive, gone, off := uuid.New(), uuid.New(), uuid.New()
	tok := func(id uuid.UUID) string {
		s, err := jm.GenerateToken(id, "a@b.co", "A")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	lookups := 0
	check := func(_ context.Context, id uuid.UUID) (bool, error) {
		lookups++
		switch id {
		case alive:
			return true, nil
		case off:
			return false, nil // exists but disabled
		}
		return false, nil // deleted
	}
	h := AuthMiddlewareWithAccounts(jm, check)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	if c := call(t, h, tok(alive)); c != 200 {
		t.Fatalf("active account: got %d", c)
	}
	if c := call(t, h, tok(gone)); c != 401 {
		t.Fatalf("deleted account must be 401, got %d", c)
	}
	if c := call(t, h, tok(off)); c != 401 {
		t.Fatalf("disabled account must be 401, got %d", c)
	}
	before := lookups
	call(t, h, tok(alive))
	call(t, h, tok(gone))
	if lookups != before {
		t.Fatalf("lookups should be cached within the TTL: %d -> %d", before, lookups)
	}
	if c := call(t, h, "not-a-token"); c != 401 {
		t.Fatalf("garbage token: got %d", c)
	}
}

func TestAccountLookupFailureIsRefusedNotWavedThrough(t *testing.T) {
	jm := auth.NewJWTManager("test-secret-test-secret-test-secret", time.Hour)
	s, _ := jm.GenerateToken(uuid.New(), "a@b.co", "A")
	h := AuthMiddlewareWithAccounts(jm, func(context.Context, uuid.UUID) (bool, error) { return false, errors.New("db down") })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	if c := call(t, h, s); c != 503 {
		t.Fatalf("lookup failure must be 503, got %d", c)
	}
}

func TestNilCheckKeepsOldBehaviour(t *testing.T) {
	jm := auth.NewJWTManager("test-secret-test-secret-test-secret", time.Hour)
	s, _ := jm.GenerateToken(uuid.New(), "a@b.co", "A")
	h := AuthMiddleware(jm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	if c := call(t, h, s); c != 200 {
		t.Fatalf("got %d", c)
	}
}
