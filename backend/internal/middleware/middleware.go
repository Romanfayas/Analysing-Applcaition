package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/halal-equity/backend/internal/auth"
)

type contextKey string

const UserContextKey contextKey = "user"

// AccountCheck reports whether the account behind a token still exists and is active.
type AccountCheck func(ctx context.Context, id uuid.UUID) (active bool, err error)

// accountCacheTTL bounds how long a deleted or disabled account's token keeps working.
const accountCacheTTL = 30 * time.Second

type accountEntry struct {
	active bool
	until  time.Time
}

// AuthMiddleware validates JWT tokens and adds user info to context. It trusts the token alone.
func AuthMiddleware(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return AuthMiddlewareWithAccounts(jwtManager, nil)
}

// AuthMiddlewareWithAccounts also rejects a validly signed token whose account was deleted or deactivated, so a stale browser
// session gets a clean 401 (and is sent to the login page) instead of failing deeper with foreign-key errors.
// If the account lookup itself fails the request is refused with 503 rather than waved through.
func AuthMiddlewareWithAccounts(jwtManager *auth.JWTManager, check AccountCheck) func(http.Handler) http.Handler {
	var mu sync.Mutex
	cache := map[uuid.UUID]accountEntry{}
	now := time.Now
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtManager.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			if check != nil {
				mu.Lock()
				e, hit := cache[claims.UserID]
				mu.Unlock()
				if !hit || now().After(e.until) {
					active, err := check(r.Context(), claims.UserID)
					if err != nil {
						http.Error(w, `{"error":"could not verify the account"}`, http.StatusServiceUnavailable)
						return
					}
					e = accountEntry{active: active, until: now().Add(accountCacheTTL)}
					mu.Lock()
					cache[claims.UserID] = e
					mu.Unlock()
				}
				if !e.active {
					http.Error(w, `{"error":"this account no longer exists or is disabled: please sign in again"}`, http.StatusUnauthorized)
					return
				}
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves the authenticated user claims from context.
func GetUserFromContext(ctx context.Context) *auth.Claims {
	claims, ok := ctx.Value(UserContextKey).(*auth.Claims)
	if !ok {
		return nil
	}
	return claims
}

// RequestLogger logs HTTP requests with structured fields.
func RequestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(ww, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.statusCode).
				Dur("duration", time.Since(start)).
				Str("remote_addr", r.RemoteAddr).
				Str("user_agent", r.UserAgent()).
				Msg("request")
		})
	}
}

// RateLimiter creates a rate limiting middleware.
func RateLimiter(rps int, burst int) func(http.Handler) http.Handler {
	return httprate.LimitByIP(rps, time.Second)
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
