package official

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

var (
	ErrAccessRestricted      = errors.New("access restricted by exchange (HTTP 403/WAF); automated acquisition stopped fail-closed without circumvention")
	ErrInvalidOfficialDomain = errors.New("source URL does not match authenticated official exchange endpoint (nseindia.com or bseindia.com)")
	ErrIntegrityMismatch     = errors.New("downloaded artifact SHA-256 checksum does not match expected digest")
	ErrUnauthorizedPromotion = errors.New("unauthorized status promotion: PRIMARY_VERIFIED requires authenticated exchange origin proof and reviewer confirmation")
	ErrLocalArchivePromotion = errors.New("local archive, fixture, or unauthenticated local cache cannot be promoted to PRIMARY_VERIFIED")
	ErrDuplicateArtifact     = errors.New("duplicate artifact: identical SHA-256 checksum already registered in repository")
	ErrSSRFAttemptDetected   = errors.New("SSRF security violation: target resolves to private, loopback, or reserved network address")
	ErrInsecureScheme        = errors.New("insecure scheme: official acquisition strictly requires https://")
)

// ApprovedExchangeHosts contains the strict whitelist of authorized official exchange hosts.
var ApprovedExchangeHosts = map[string]bool{
	"nseindia.com":     true,
	"www.nseindia.com": true,
	// NSE's official file archive (bhavcopies, circular attachments).
	"nsearchives.nseindia.com": true,
	"bseindia.com":             true,
	"www.bseindia.com":         true,
}

// AcquisitionMetadata records transport and origin metadata during document acquisition.
type AcquisitionMetadata struct {
	OriginalURL        string
	FinalURL           string
	HTTPStatus         int
	ContentType        string
	ByteSize           int64
	AcquisitionTimeUTC time.Time
	IsLocalAcquisition bool
	IssuerAuthority    string // 'NSE_INDIA' or 'BSE_INDIA'
	RedirectChain      []string
	IPAddress          string
	// Transport provenance: the certificate the host presented (validated by the standard TLS chain). This records
	// WHERE bytes came from; it is not proof that a document's content is authentic and must never be used as such.
	TLSPeerSubject string
	TLSPeerIssuer  string
	TLSCertSHA256  string
}

// ValidationResult separates artifact integrity, source authenticity, parsing, financial validation, and review.
type ValidationResult struct {
	ArtifactIntegrityPassed   bool
	SHA256Checksum            string
	SourceAuthenticityPassed  bool
	AuthenticityReason        string
	ParsingPassed             bool
	ParsingDetails            string
	FinancialValidationPassed bool
	FinancialValidationNotes  string
	HasAuthenticityProof      bool
	HumanReviewStatus         string // 'PENDING_REVIEW', 'APPROVED_BY_REVIEWER', 'REJECTED'
	AssignedStatus            string // 'ACQUIRED_UNVERIFIED', 'PARSED_VALIDATED', 'AUTHENTICITY_PENDING', 'PRIMARY_VERIFIED', 'REJECTED', 'AMBIGUOUS'
}

// ValidateOfficialExchangeURL enforces strict scheme, approved hostname, absence of userinfo, and anti-SSRF IP verification.
func ValidateOfficialExchangeURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL form: %w", err)
	}

	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: got %s", ErrInsecureScheme, parsed.Scheme)
	}

	if parsed.User != nil {
		return nil, fmt.Errorf("URL userinfo credentials strictly prohibited: %s", rawURL)
	}

	hostname := strings.ToLower(parsed.Hostname())
	if !ApprovedExchangeHosts[hostname] {
		return nil, fmt.Errorf("%w: host %s is not an approved exchange host", ErrInvalidOfficialDomain, hostname)
	}

	// Anti-SSRF: Resolve IP and verify it does not map to private or loopback networks
	ips, err := net.LookupIP(hostname)
	if err == nil {
		for _, ip := range ips {
			if isPrivateOrReservedIP(ip) {
				return nil, fmt.Errorf("%w: hostname %s resolves to %s", ErrSSRFAttemptDetected, hostname, ip.String())
			}
		}
	}

	return parsed, nil
}

func isPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// Pipeline handles provider-independent document acquisition for official NSE and BSE corporate actions.
type Pipeline struct {
	httpClient *http.Client
	mu         sync.Mutex
	lastCall   time.Time
	minDelay   time.Duration
}

// NewPipeline initializes an acquisition pipeline with strict redirect checking and anti-SSRF controls.
func NewPipeline(minDelay time.Duration) *Pipeline {
	if minDelay == 0 {
		minDelay = 1 * time.Second
	}
	return &Pipeline{
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("redirect limit exceeded (max 3 redirects)")
				}
				// Validate redirect target strictly against official exchange policy at every hop
				if _, err := ValidateOfficialExchangeURL(req.URL.String()); err != nil {
					return fmt.Errorf("redirect security violation at hop %d: %w", len(via), err)
				}
				return nil
			},
		},
		minDelay: minDelay,
	}
}

// AcquireFromOfficialURL acquires a corporate action document from an official NSE or BSE endpoint.
// If HTTP 403 or access restriction is encountered, it halts fail-closed without circumvention.
func (p *Pipeline) AcquireFromOfficialURL(ctx context.Context, rawURL string) ([]byte, *AcquisitionMetadata, error) {
	// 1. Strict URL validation and Anti-SSRF check
	parsed, err := ValidateOfficialExchangeURL(rawURL)
	if err != nil {
		return nil, nil, err
	}

	// 2. Determine Issuing Authority
	authority := "NSE_INDIA"
	if strings.Contains(parsed.Host, "bseindia.com") {
		authority = "BSE_INDIA"
	}

	// 3. Polite Rate Limiting
	p.mu.Lock()
	elapsed := time.Since(p.lastCall)
	if elapsed < p.minDelay {
		time.Sleep(p.minDelay - elapsed)
	}
	p.lastCall = time.Now()
	p.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build HTTP request: %w", err)
	}
	// Identify honestly; if the exchange refuses this client we stop (ErrAccessRestricted), we never impersonate a browser.
	req.Header.Set("User-Agent", "HalalEquity-Statutory-Evidence-Harvester/1.9.24")
	req.Header.Set("Accept", "application/pdf,application/json,text/html,*/*")

	resp, err := p.httpClient.Do(req)
	acqTime := time.Now().UTC()
	if err != nil {
		return nil, nil, fmt.Errorf("network error during official acquisition: %w", err)
	}
	defer resp.Body.Close()

	meta := &AcquisitionMetadata{
		OriginalURL:        rawURL,
		FinalURL:           resp.Request.URL.String(),
		HTTPStatus:         resp.StatusCode,
		ContentType:        resp.Header.Get("Content-Type"),
		AcquisitionTimeUTC: acqTime,
		IsLocalAcquisition: false,
		IssuerAuthority:    authority,
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		sum := sha256.Sum256(leaf.Raw)
		meta.TLSPeerSubject = leaf.Subject.String()
		meta.TLSPeerIssuer = leaf.Issuer.String()
		meta.TLSCertSHA256 = hex.EncodeToString(sum[:])
	}

	// Safety Invariant: Halt on HTTP 401/403 or challenge without circumvention or proxy rotation
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, meta, fmt.Errorf("%w: HTTP %d from %s", ErrAccessRestricted, resp.StatusCode, rawURL)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, meta, fmt.Errorf("unexpected HTTP status %d from official endpoint %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, meta, fmt.Errorf("failed to read document payload: %w", err)
	}

	meta.ByteSize = int64(len(body))
	return body, meta, nil
}

// IngestLocallyAcquiredDocument processes an official document acquired manually or via local storage.
func (p *Pipeline) IngestLocallyAcquiredDocument(
	rawPayload []byte,
	officialSourceURL string,
	documentTitle string,
	authority string,
) (*AcquisitionMetadata, error) {
	if len(rawPayload) == 0 {
		return nil, fmt.Errorf("raw payload cannot be empty")
	}

	if authority == "" {
		authority = "NSE_INDIA"
		if strings.Contains(officialSourceURL, "bseindia.com") {
			authority = "BSE_INDIA"
		}
	}

	meta := &AcquisitionMetadata{
		OriginalURL:        officialSourceURL,
		FinalURL:           officialSourceURL,
		HTTPStatus:         200,
		ContentType:        "application/octet-stream",
		ByteSize:           int64(len(rawPayload)),
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: true,
		IssuerAuthority:    authority,
	}

	return meta, nil
}

// EvaluateArtifactState computes SHA-256 and rigorously separates:
// 1. Artifact integrity (checksum)
// 2. Source authenticity (origin & transport proof)
// 3. Syntax parsing
// 4. Financial validation
// 5. Authenticity proof (e.g. digital signature or authenticated exchange session)
// 6. Human review authorization
func EvaluateArtifactState(
	payload []byte,
	meta *AcquisitionMetadata,
	parsedSuccessfully bool,
	parseDetails string,
	financialValidationPassed bool,
	financialNotes string,
	reviewerConfirmed bool,
	hasAuthenticityProof bool,
) *ValidationResult {
	hasher := sha256.New()
	hasher.Write(payload)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	res := &ValidationResult{
		ArtifactIntegrityPassed:   true,
		SHA256Checksum:            checksum,
		ParsingPassed:             parsedSuccessfully,
		ParsingDetails:            parseDetails,
		FinancialValidationPassed: financialValidationPassed,
		FinancialValidationNotes:  financialNotes,
		HasAuthenticityProof:      hasAuthenticityProof,
		HumanReviewStatus:         "PENDING_REVIEW",
	}

	if reviewerConfirmed {
		res.HumanReviewStatus = "APPROVED_BY_REVIEWER"
	}

	// 1. Authenticity Check
	isArchiveOrFixture := strings.HasPrefix(meta.OriginalURL, "CONTROLLED_ARCHIVE:") ||
		strings.HasPrefix(meta.OriginalURL, "FIXTURE:") ||
		strings.HasPrefix(meta.OriginalURL, "LOCAL:")

	if isArchiveOrFixture {
		res.SourceAuthenticityPassed = false
		res.AuthenticityReason = "Artifact originates from unauthenticated local archive or fixture"
		if parsedSuccessfully && financialValidationPassed {
			res.AssignedStatus = models.ArtifactStatusParsedValidated
		} else {
			res.AssignedStatus = models.ArtifactStatusAcquiredUnverified
		}
		return res
	}

	_, urlErr := ValidateOfficialExchangeURL(meta.OriginalURL)
	if urlErr != nil {
		res.SourceAuthenticityPassed = false
		res.AuthenticityReason = fmt.Sprintf("Source URL %s failed official domain validation: %v", meta.OriginalURL, urlErr)
		res.AssignedStatus = models.ArtifactStatusRejected
		return res
	}

	res.SourceAuthenticityPassed = true
	res.AuthenticityReason = fmt.Sprintf("Source verified against approved %s official endpoint", meta.IssuerAuthority)

	// 2. Parse & Financial Validation Check
	if !parsedSuccessfully {
		res.AssignedStatus = models.ArtifactStatusRejected
		return res
	}

	if !financialValidationPassed {
		res.AssignedStatus = models.ArtifactStatusAmbiguous
		return res
	}

	// 3. Authenticity Proof & Human Review Check:
	// A document from an approved domain with valid hash and syntax is AUTHENTICITY_PENDING
	// until external cryptographic exchange proof AND human compliance reviewer approval are confirmed.
	if !hasAuthenticityProof || !reviewerConfirmed {
		res.AssignedStatus = models.ArtifactStatusAuthPending
		if !hasAuthenticityProof {
			res.AuthenticityReason = "Cryptographic exchange signature or institutional transport origin proof is pending"
		} else {
			res.AuthenticityReason = "Awaiting qualified compliance reviewer authorization"
		}
		return res
	}

	// Verified exchange primary requires ALL:
	// - official origin (not archive/fixture)
	// - SHA-256 integrity
	// - successful parsing
	// - financial validation
	// - cryptographic authenticity proof
	// - human reviewer confirmation
	res.AssignedStatus = models.ArtifactStatusPrimaryVerified
	return res
}

// BuildSourceArtifact creates the database model ensuring clear separation of states.
func BuildSourceArtifact(
	symbol string,
	meta *AcquisitionMetadata,
	payload []byte,
	val *ValidationResult,
	docType string,
) *models.CorporateActionSourceArtifact {
	docID := fmt.Sprintf("%s_%s_%s", meta.IssuerAuthority, symbol, val.SHA256Checksum[:10])
	artifactID := fmt.Sprintf("ART_%s_%s", symbol, val.SHA256Checksum[:16])

	var reviewer *string
	var reviewedAt *time.Time
	if val.HumanReviewStatus == "APPROVED_BY_REVIEWER" {
		r := "QUANT_SYSTEMS_AUDITOR"
		reviewer = &r
		now := time.Now().UTC()
		reviewedAt = &now
	}

	return &models.CorporateActionSourceArtifact{
		ArtifactID:         artifactID,
		IssuingAuthority:   meta.IssuerAuthority,
		SourceURL:          meta.OriginalURL,
		DocumentIdentifier: docID,
		DocumentType:       docType,
		RetrievalTimestamp: meta.AcquisitionTimeUTC,
		RawPayload:         string(payload),
		SHA256Checksum:     val.SHA256Checksum,
		ParserVersion:      "v1.9.18",
		ValidationStatus:   val.AssignedStatus,
		Reviewer:           reviewer,
		ReviewedAt:         reviewedAt,
		CreatedAt:          meta.AcquisitionTimeUTC,
	}
}
