package evidence

import (
	"bytes"
	"compress/zlib"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Signature is what could be established about one embedded PDF signature.
//
// Intact means the PKCS#7 signature verifies over exactly the byte ranges it declares. It does NOT mean the
// signer is trusted: the certificate chain shipped inside the document is reported but not validated against an
// independently obtained trust anchor, and revocation is not checked.
type Signature struct {
	Intact          bool
	CoversDocument  bool // the signed byte ranges span the whole file (nothing appended after signing)
	SignerSubject   string
	SignerOrg       string
	ChainRoot       string
	SigningTime     string
	VerifierMessage string
}

var (
	reByteRange = regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	reSignTime  = regexp.MustCompile(`(?s)signingTime.*?UTCTIME:([^\n]+)`)
)

// VerifyPDFSignatures checks every embedded signature with the OpenSSL CLI.
func VerifyPDFSignatures(pdf []byte) ([]Signature, error) {
	if _, err := exec.LookPath("openssl"); err != nil {
		return nil, fmt.Errorf("openssl CLI is required for signature checks: %w", err)
	}
	var out []Signature
	for _, m := range reByteRange.FindAllSubmatch(pdf, -1) {
		var r [4]int
		for i := range r {
			r[i], _ = strconv.Atoi(string(m[i+1]))
		}
		a, b, c, d := r[0], r[1], r[2], r[3]
		if a < 0 || b < 0 || c < a+b || d < 0 || c+d > len(pdf) {
			out = append(out, Signature{VerifierMessage: "byte range outside the file"})
			continue
		}
		hexs := strings.TrimRight(strings.Trim(string(bytes.TrimSpace(pdf[a+b:c])), "<>"), "0")
		if len(hexs)%2 == 1 {
			hexs += "0"
		}
		der, err := hex.DecodeString(hexs)
		if err != nil || len(der) == 0 {
			out = append(out, Signature{VerifierMessage: "signature contents are not valid hex/DER"})
			continue
		}
		signed := append(append([]byte{}, pdf[a:a+b]...), pdf[c:c+d]...)
		s, err := verifyCMS(der, signed)
		if err != nil {
			return nil, err
		}
		s.CoversDocument = a == 0 && c+d == len(pdf)
		out = append(out, s)
	}
	return out, nil
}

func verifyCMS(der, content []byte) (Signature, error) {
	dir, err := os.MkdirTemp("", "pdfsig")
	if err != nil {
		return Signature{}, err
	}
	defer os.RemoveAll(dir)
	sig, cnt := filepath.Join(dir, "s.der"), filepath.Join(dir, "c.bin")
	if err := os.WriteFile(sig, der, 0o600); err != nil {
		return Signature{}, err
	}
	if err := os.WriteFile(cnt, content, 0o600); err != nil {
		return Signature{}, err
	}
	var s Signature
	// -noverify: check the signature over the bytes, deliberately NOT trusting the embedded chain.
	signerPath := filepath.Join(dir, "signer.pem")
	v, _ := exec.Command("openssl", "cms", "-verify", "-binary", "-inform", "DER", "-in", sig, "-content", cnt,
		"-noverify", "-signer", signerPath, "-out", os.DevNull).CombinedOutput()
	s.VerifierMessage = strings.TrimSpace(string(v))
	s.Intact = strings.Contains(s.VerifierMessage, "Verification successful")

	pemCerts, _ := exec.Command("openssl", "pkcs7", "-inform", "DER", "-in", sig, "-print_certs").Output()
	var certs []*x509.Certificate
	for rest := pemCerts; ; {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if c, err := x509.ParseCertificate(blk.Bytes); err == nil {
			certs = append(certs, c)
		}
	}
	for _, c := range certs {
		if c.Subject.String() == c.Issuer.String() {
			s.ChainRoot = c.Subject.String() // embedded in the document: reported, never trusted
		}
	}
	// The signer is the certificate named in the CMS SignerInfo, as reported by OpenSSL (not inferred).
	if raw, err := os.ReadFile(signerPath); err == nil {
		if blk, _ := pem.Decode(raw); blk != nil {
			if c, err := x509.ParseCertificate(blk.Bytes); err == nil {
				s.SignerSubject, s.SignerOrg = c.Subject.CommonName, strings.Join(c.Subject.Organization, ", ")
			}
		}
	}
	if p, err := exec.Command("openssl", "cms", "-cmsout", "-print", "-inform", "DER", "-in", sig).Output(); err == nil {
		if m := reSignTime.FindSubmatch(p); m != nil {
			s.SigningTime = strings.TrimSpace(string(m[1]))
		}
	}
	return s, nil
}

var (
	reStream  = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	reTj      = regexp.MustCompile(`\(((?:\\.|[^\\)])*)\)\s*Tj`)
	reTJ      = regexp.MustCompile(`(?s)\[(.*?)\]\s*TJ`)
	reLiteral = regexp.MustCompile(`\(((?:\\.|[^\\)])*)\)`)
)

// ExtractText pulls literal text out of Flate-compressed content streams. It is deliberately simple; when it
// finds nothing usable the content checks report "unconfirmed" and a human must read the document.
func ExtractText(pdf []byte) string {
	var b strings.Builder
	for _, m := range reStream.FindAllSubmatch(pdf, -1) {
		zr, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(zr, 8<<20))
		zr.Close()
		if err != nil && len(raw) == 0 {
			continue
		}
		for _, t := range reTj.FindAllSubmatch(raw, -1) {
			b.Write(t[1])
			b.WriteByte(' ')
		}
		for _, arr := range reTJ.FindAllSubmatch(raw, -1) {
			for _, t := range reLiteral.FindAllSubmatch(arr[1], -1) {
				b.Write(t[1])
			}
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// ContentChecks looks for the action's identifying facts in a document's text. Whitespace is removed before
// matching because extracted PDF text splits words unpredictably.
func ContentChecks(text string, c Classified) map[string]any {
	norm := strings.ToLower(strings.Join(strings.Fields(text), ""))
	has := func(xs ...string) bool {
		for _, x := range xs {
			if x != "" && strings.Contains(norm, strings.ToLower(strings.ReplaceAll(x, " ", ""))) {
				return true
			}
		}
		return false
	}
	ex := c.Ex
	dates := []string{ex.Format("02-Jan-2006"), ex.Format("2-Jan-2006"), ex.Format("January 2, 2006"), ex.Format("January 02, 2006"),
		ex.Format("02/01/2006"), ex.Format("02.01.2006"), ex.Format("2 January 2006"), ex.Format("02 January 2006"),
		ex.Format("Monday, January 2, 2006"), ex.Format("2006-01-02")}
	out := map[string]any{"text_extracted_chars": len(norm), "date_found": has(dates...)}
	switch c.Class {
	case ClassBonus:
		if m := reBonus.FindStringSubmatch(c.Subject); m != nil {
			out["ratio_found"] = has(m[1]+":"+m[2], m[1]+" : "+m[2])
		}
		out["keyword_found"] = has("bonus")
	case ClassSplit:
		out["keyword_found"] = has("sub-division", "subdivision", "split")
	case ClassDemerger:
		out["keyword_found"] = has("demerger", "schemeofarrangement", "arrangement")
	}
	return out
}

// DocumentVerdict combines signature and content results. It never returns a "verified" state.
func DocumentVerdict(sigs []Signature, checks map[string]any) string {
	if n, _ := checks["text_extracted_chars"].(int); n == 0 && len(sigs) == 0 {
		return "UNREADABLE"
	}
	if len(sigs) == 0 {
		return "UNSIGNED_PENDING_REVIEW"
	}
	for _, s := range sigs {
		if !s.Intact {
			return "SIGNATURE_INVALID"
		}
	}
	matched := checks["date_found"] == true && checks["keyword_found"] == true
	if r, ok := checks["ratio_found"]; ok {
		matched = matched && r == true
	}
	if matched {
		return "SIGNATURE_INTACT_CONTENT_MATCHED_PENDING_REVIEW"
	}
	return "SIGNATURE_INTACT_CONTENT_UNCONFIRMED_PENDING_REVIEW"
}
