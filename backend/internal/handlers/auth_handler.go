package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/auth"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/pkg/response"
)

const (
	minPasswordLen = 10
	// bcrypt silently ignores bytes past 72; reject instead of truncating a secret.
	maxPasswordBytes = 72
	maxBodyBytes     = 8 << 10
)

// userStore is the persistence surface the auth handler needs (satisfied by *database.DB).
type userStore interface {
	CreateUser(ctx context.Context, email, name, passwordHash string) (*database.UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*database.UserRecord, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*database.UserRecord, error)
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	users             userStore
	jwt               *auth.JWTManager
	expiry            time.Duration
	allowRegistration bool
	// dummyHash is compared against when an account does not exist so that
	// "unknown email" and "wrong password" take the same time (no user enumeration).
	dummyHash string
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(users userStore, jwt *auth.JWTManager, expiry time.Duration, allowRegistration bool) *AuthHandler {
	dummy, _ := auth.HashPassword(uuid.NewString())
	return &AuthHandler{users: users, jwt: jwt, expiry: expiry, allowRegistration: allowRegistration, dummyHash: dummy}
}

// Routes registers auth routes.
func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.RefreshToken)
	return r
}

// RegisterRequest represents a registration payload.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginRequest represents a login payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authUser struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type authResult struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      authUser  `json:"user"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.ValidationError(w, "invalid request body")
		return false
	}
	return true
}

func (h *AuthHandler) issue(w http.ResponseWriter, status int, u *database.UserRecord) {
	token, err := h.jwt.GenerateToken(u.ID, u.Email, u.Name)
	if err != nil {
		log.Error().Err(err).Msg("token signing failed")
		response.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	response.JSON(w, status, authResult{
		Token:     token,
		ExpiresAt: time.Now().Add(h.expiry).UTC(),
		User:      authUser{ID: u.ID, Email: u.Email, Name: u.Name},
	})
}

// Register creates a new user account and signs the user in.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.allowRegistration {
		response.Error(w, http.StatusForbidden, "self-service registration is disabled")
		return
	}
	var req RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	if req.Email == "" || req.Password == "" || req.Name == "" {
		response.ValidationError(w, "email, password, and name are required")
		return
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email {
		response.ValidationError(w, "email is not a valid address")
		return
	}
	if utf8.RuneCountInString(req.Name) > 120 {
		response.ValidationError(w, "name is too long")
		return
	}
	if utf8.RuneCountInString(req.Password) < minPasswordLen {
		response.ValidationError(w, "password must be at least 10 characters")
		return
	}
	if len(req.Password) > maxPasswordBytes {
		response.ValidationError(w, "password must be at most 72 bytes")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}
	user, err := h.users.CreateUser(r.Context(), req.Email, req.Name, hash)
	if errors.Is(err, database.ErrUserExists) {
		response.Error(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("create user failed")
		response.Error(w, http.StatusInternalServerError, "could not create account")
		return
	}
	h.issue(w, http.StatusCreated, user)
}

// Login authenticates a user and returns a JWT token.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		response.ValidationError(w, "email and password are required")
		return
	}

	user, err := h.users.GetUserByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, database.ErrUserNotFound) {
		log.Error().Err(err).Msg("login lookup failed")
		response.Error(w, http.StatusInternalServerError, "login failed")
		return
	}
	hash := h.dummyHash
	if user != nil {
		hash = user.PasswordHash
	}
	passwordOK := auth.CheckPassword(req.Password, hash)
	if user == nil || !user.IsActive || !passwordOK {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	h.issue(w, http.StatusOK, user)
}

// RefreshToken exchanges a still-valid token for a fresh one. The account must still be active.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		response.Unauthorized(w)
		return
	}
	claims, err := h.jwt.ValidateToken(parts[1])
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}
	user, err := h.users.GetUserByID(r.Context(), claims.UserID)
	if err != nil || !user.IsActive {
		response.Error(w, http.StatusUnauthorized, "account unavailable")
		return
	}
	h.issue(w, http.StatusOK, user)
}
