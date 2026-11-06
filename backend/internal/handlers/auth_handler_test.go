package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/auth"
	"github.com/halal-equity/backend/internal/database"
)

type fakeUserStore struct {
	mu    sync.Mutex
	users map[string]*database.UserRecord
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{users: map[string]*database.UserRecord{}}
}

func (f *fakeUserStore) CreateUser(_ context.Context, email, name, hash string) (*database.UserRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	email = database.NormalizeEmail(email)
	if _, ok := f.users[email]; ok {
		return nil, database.ErrUserExists
	}
	u := &database.UserRecord{ID: uuid.New(), Email: email, Name: name, PasswordHash: hash, IsActive: true}
	f.users[email] = u
	return u, nil
}

func (f *fakeUserStore) GetUserByEmail(_ context.Context, email string) (*database.UserRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[database.NormalizeEmail(email)]; ok {
		return u, nil
	}
	return nil, database.ErrUserNotFound
}

func (f *fakeUserStore) GetUserByID(_ context.Context, id uuid.UUID) (*database.UserRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, database.ErrUserNotFound
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func newAuthFixture(allowRegistration bool) (*AuthHandler, *fakeUserStore, *auth.JWTManager) {
	store := newFakeUserStore()
	jwt := auth.NewJWTManager("unit-test-secret", time.Hour)
	return NewAuthHandler(store, jwt, time.Hour, allowRegistration), store, jwt
}

func post(h *AuthHandler, path string, body any, headers map[string]string) (*httptest.ResponseRecorder, envelope) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

const goodPassword = "correct horse battery"

func TestAuth_RegisterThenLoginIssuesVerifiableToken(t *testing.T) {
	h, _, jwt := newAuthFixture(true)

	rec, env := post(h, "/register", RegisterRequest{Email: "  Ada@Example.com ", Password: goodPassword, Name: "Ada"}, nil)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	rec, env = post(h, "/login", LoginRequest{Email: "ada@example.com", Password: goodPassword}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var res authResult
	require.NoError(t, json.Unmarshal(env.Data, &res))
	claims, err := jwt.ValidateToken(res.Token)
	require.NoError(t, err)
	assert.Equal(t, "ada@example.com", claims.Email)
	assert.Equal(t, res.User.ID, claims.UserID)
	assert.NotContains(t, rec.Body.String(), "password", "response must never expose credential material")
}

func TestAuth_RegisterRejectsWeakAndMalformedInput(t *testing.T) {
	h, _, _ := newAuthFixture(true)
	cases := map[string]RegisterRequest{
		"short password":     {Email: "a@b.co", Password: "short", Name: "A"},
		"too long (bcrypt)":  {Email: "a@b.co", Password: strings.Repeat("x", 73), Name: "A"},
		"bad email":          {Email: "not-an-email", Password: goodPassword, Name: "A"},
		"display-name email": {Email: "Bob <bob@x.io>", Password: goodPassword, Name: "A"},
		"missing name":       {Email: "a@b.co", Password: goodPassword},
	}
	for name, body := range cases {
		rec, _ := post(h, "/register", body, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, name)
	}
}

func TestAuth_DuplicateRegistrationConflictsCaseInsensitively(t *testing.T) {
	h, _, _ := newAuthFixture(true)
	rec, _ := post(h, "/register", RegisterRequest{Email: "dup@x.io", Password: goodPassword, Name: "D"}, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	rec, _ = post(h, "/register", RegisterRequest{Email: "DUP@x.io", Password: goodPassword, Name: "D2"}, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestAuth_RegistrationCanBeDisabled(t *testing.T) {
	h, _, _ := newAuthFixture(false)
	rec, _ := post(h, "/register", RegisterRequest{Email: "a@b.co", Password: goodPassword, Name: "A"}, nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAuth_LoginDoesNotRevealWhetherAccountExists(t *testing.T) {
	h, _, _ := newAuthFixture(true)
	post(h, "/register", RegisterRequest{Email: "real@x.io", Password: goodPassword, Name: "R"}, nil)

	wrongPw, e1 := post(h, "/login", LoginRequest{Email: "real@x.io", Password: "wrong password!!"}, nil)
	noUser, e2 := post(h, "/login", LoginRequest{Email: "ghost@x.io", Password: "wrong password!!"}, nil)
	assert.Equal(t, http.StatusUnauthorized, wrongPw.Code)
	assert.Equal(t, http.StatusUnauthorized, noUser.Code)
	assert.Equal(t, e1.Error, e2.Error, "identical error for unknown account and wrong password")
}

func TestAuth_InactiveAccountCannotLoginOrRefresh(t *testing.T) {
	h, store, jwt := newAuthFixture(true)
	_, env := post(h, "/register", RegisterRequest{Email: "off@x.io", Password: goodPassword, Name: "O"}, nil)
	var res authResult
	require.NoError(t, json.Unmarshal(env.Data, &res))

	store.users["off@x.io"].IsActive = false

	rec, _ := post(h, "/login", LoginRequest{Email: "off@x.io", Password: goodPassword}, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec, _ = post(h, "/refresh", nil, map[string]string{"Authorization": "Bearer " + res.Token})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	_ = jwt
}

func TestAuth_RefreshRequiresValidToken(t *testing.T) {
	h, _, _ := newAuthFixture(true)
	_, env := post(h, "/register", RegisterRequest{Email: "r@x.io", Password: goodPassword, Name: "R"}, nil)
	var res authResult
	require.NoError(t, json.Unmarshal(env.Data, &res))

	rec, _ := post(h, "/refresh", nil, map[string]string{"Authorization": "Bearer " + res.Token})
	assert.Equal(t, http.StatusOK, rec.Code)

	for name, hdr := range map[string]string{"none": "", "garbage": "Bearer not.a.jwt", "scheme": "Basic abc"} {
		rec, _ = post(h, "/refresh", nil, map[string]string{"Authorization": hdr})
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
	}

	other := auth.NewJWTManager("a-different-secret", time.Hour)
	forged, _ := other.GenerateToken(res.User.ID, "r@x.io", "R")
	rec, _ = post(h, "/refresh", nil, map[string]string{"Authorization": "Bearer " + forged})
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "token signed with another key must be rejected")
}

func TestAuth_RejectsUnknownFieldsAndOversizedBodies(t *testing.T) {
	h, _, _ := newAuthFixture(true)
	rec, _ := post(h, "/login", map[string]any{"email": "a@b.co", "password": "x", "is_admin": true}, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec, _ = post(h, "/login", map[string]any{"email": "a@b.co", "password": strings.Repeat("x", 20<<10)}, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
