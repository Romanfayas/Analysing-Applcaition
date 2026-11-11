package handlers

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
)

// symbolParam returns the {symbol} path parameter, percent-decoded. chi hands back the raw segment, so "M&M" arrives as "M%26M"
// and the lookup missed: every stock whose ticker contains & (M&M, M&MFIN) showed "failed to load stock data".
func symbolParam(r *http.Request) string {
	raw := chi.URLParam(r, "symbol")
	if s, err := url.PathUnescape(raw); err == nil {
		return s
	}
	return raw
}
