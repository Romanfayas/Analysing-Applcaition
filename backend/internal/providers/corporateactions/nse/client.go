package nse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// Client handles rate-limited, resilient HTTP communication with the official NSE website.
type Client struct {
	httpClient *http.Client
	baseURL    string
	mu         sync.Mutex
	hasCookies bool
	lastCall   time.Time
	minDelay   time.Duration
}

// NewClient initializes an NSE client with a cookie jar and polite rate limiting.
func NewClient(minDelay time.Duration) *Client {
	jar, _ := cookiejar.New(nil)
	if minDelay == 0 {
		minDelay = 600 * time.Millisecond
	}
	return &Client{
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 20 * time.Second,
		},
		baseURL:  "https://www.nseindia.com",
		minDelay: minDelay,
	}
}

// ensureSession initializes browser cookies from the NSE homepage.
func (c *Client) ensureSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasCookies {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach NSE homepage for session cookie handshake: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		c.hasCookies = true
		return nil
	}
	return fmt.Errorf("unexpected status from NSE homepage: %d", resp.StatusCode)
}

// FetchCorporateActionsRaw fetches raw corporate action JSON for a symbol with bounded exponential backoff.
func (c *Client) FetchCorporateActionsRaw(ctx context.Context, symbol string) (string, []byte, int, error) {
	url := fmt.Sprintf("%s/api/corporates-corporateActions?index=equities&symbol=%s", c.baseURL, symbol)

	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Rate-limiting throttle
		c.mu.Lock()
		elapsed := time.Since(c.lastCall)
		if elapsed < c.minDelay {
			time.Sleep(c.minDelay - elapsed)
		}
		c.lastCall = time.Now()
		c.mu.Unlock()

		if err := c.ensureSession(ctx); err != nil {
			if attempt == maxRetries {
				return url, nil, 0, err
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return url, nil, 0, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", "https://www.nseindia.com/")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return url, nil, 0, fmt.Errorf("network error on attempt %d: %w", attempt, err)
			}
			jitter := time.Duration(rand.Intn(300)) * time.Millisecond
			time.Sleep(backoff + jitter)
			backoff *= 2
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return url, nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			return url, body, resp.StatusCode, nil
		}

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			// Session expired or blocked, reset session cookie
			c.mu.Lock()
			c.hasCookies = false
			c.mu.Unlock()
		}

		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < maxRetries {
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(backoff + jitter)
			backoff *= 2
			continue
		}

		return url, body, resp.StatusCode, fmt.Errorf("HTTP error %d from NSE API for symbol %s", resp.StatusCode, symbol)
	}

	return url, nil, 0, fmt.Errorf("exceeded max retries for symbol %s", symbol)
}

// BuildArtifact creates an immutable CorporateActionSourceArtifact with SHA-256 checksum.
func BuildArtifact(
	symbol string,
	sourceURL string,
	rawPayload []byte,
	retrievalTime time.Time,
) *models.CorporateActionSourceArtifact {
	hasher := sha256.New()
	hasher.Write(rawPayload)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	docID := fmt.Sprintf("NSE_CA_%s_%s_%s", symbol, retrievalTime.Format("20060102"), checksum[:8])
	artifactID := fmt.Sprintf("NSE_CA_%s_%s", symbol, checksum[:16])

	return &models.CorporateActionSourceArtifact{
		ArtifactID:         artifactID,
		IssuingAuthority:   "NSE_INDIA",
		SourceURL:          sourceURL,
		DocumentIdentifier: docID,
		DocumentType:       "EXCHANGE_API_JSON",
		RetrievalTimestamp: retrievalTime,
		RawPayload:         string(rawPayload),
		SHA256Checksum:     checksum,
		ParserVersion:      "v1.8.11",
		ValidationStatus:   "RAW_RETRIEVED",
		CreatedAt:          retrievalTime,
	}
}

// DecodeRawJSON unmarshals the raw JSON array from NSE.
func DecodeRawJSON(body []byte) ([]RawNSECorporateAction, error) {
	var items []RawNSECorporateAction
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to decode NSE JSON: %w", err)
	}
	return items, nil
}
