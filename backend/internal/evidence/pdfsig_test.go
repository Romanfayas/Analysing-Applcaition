package evidence

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentsHexLen = 16384

// buildPDF returns a minimal PDF whose content stream says `text`, with an empty signature placeholder when signed.
func buildPDF(t *testing.T, text string, withSig bool) []byte {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	fmt.Fprintf(zw, "BT /F1 12 Tf (%s) Tj ET", text)
	zw.Close()
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n1 0 obj <</Length " + fmt.Sprint(z.Len()) + " /Filter /FlateDecode>>\nstream\n")
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	if withSig {
		b.WriteString("2 0 obj <</Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 0000000000 0000000000 0000000000] /Contents <")
		b.WriteString(string(bytes.Repeat([]byte("0"), contentsHexLen)))
		b.WriteString(">>>\nendobj\n")
	}
	b.WriteString("%%EOF\n")
	return b.Bytes()
}

// sign fills the placeholder with a detached CMS signature made by a throwaway key.
func sign(t *testing.T, pdf []byte, dir string) []byte {
	key, cert := filepath.Join(dir, "k.pem"), filepath.Join(dir, "c.pem")
	if _, err := os.Stat(cert); err != nil {
		out, err := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", cert,
			"-days", "1", "-subj", "/O=TEST FILER LIMITED/CN=Test Company Secretary").CombinedOutput()
		require.NoError(t, err, string(out))
	}
	open := bytes.Index(pdf, []byte("/Contents <")) + len("/Contents ")
	closeIdx := open + 1 + contentsHexLen + 1
	br := fmt.Sprintf("[0 %010d %010d %010d]", open, closeIdx, len(pdf)-closeIdx)
	pdf = bytes.Replace(pdf, []byte("[0 0000000000 0000000000 0000000000]"), []byte(br), 1)
	signed := append(append([]byte{}, pdf[:open]...), pdf[closeIdx:]...)
	in, sig := filepath.Join(dir, "in.bin"), filepath.Join(dir, "sig.der")
	require.NoError(t, os.WriteFile(in, signed, 0o600))
	out, err := exec.Command("openssl", "cms", "-sign", "-binary", "-in", in, "-signer", cert, "-inkey", key, "-outform", "DER", "-out", sig).CombinedOutput()
	require.NoError(t, err, string(out))
	der, err := os.ReadFile(sig)
	require.NoError(t, err)
	h := []byte(hex.EncodeToString(der))
	copy(pdf[open+1:], h)
	return pdf
}

func requireOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl CLI not available")
	}
}

var bonus = Classify(FeedAction{ExDate: "28-Oct-2024", Subject: "Bonus 1:1"})

func TestSignedFilingIsIntactAndContentMatched(t *testing.T) {
	requireOpenSSL(t)
	pdf := sign(t, buildPDF(t, "Record Date for the purpose of Bonus 1:1 is 28-Oct-2024", true), t.TempDir())
	sigs, err := VerifyPDFSignatures(pdf)
	require.NoError(t, err)
	require.Len(t, sigs, 1)
	assert.True(t, sigs[0].Intact, sigs[0].VerifierMessage)
	assert.True(t, sigs[0].CoversDocument)
	assert.Equal(t, "TEST FILER LIMITED", sigs[0].SignerOrg)
	assert.Equal(t, "Test Company Secretary", sigs[0].SignerSubject)
	assert.Contains(t, sigs[0].ChainRoot, "TEST FILER LIMITED", "embedded root is reported (self-signed here), never trusted")

	checks := ContentChecks(ExtractText(pdf), bonus)
	assert.Equal(t, true, checks["date_found"])
	assert.Equal(t, true, checks["ratio_found"])
	assert.Equal(t, "SIGNATURE_INTACT_CONTENT_MATCHED_PENDING_REVIEW", DocumentVerdict(sigs, checks))
}

func TestTamperedFilingFailsSignatureCheck(t *testing.T) {
	requireOpenSSL(t)
	pdf := sign(t, buildPDF(t, "Record Date for the purpose of Bonus 1:1 is 28-Oct-2024", true), t.TempDir())
	i := bytes.Index(pdf, []byte("%PDF-1.7")) + 5
	pdf[i] = '2' // edit inside the signed range
	sigs, err := VerifyPDFSignatures(pdf)
	require.NoError(t, err)
	require.Len(t, sigs, 1)
	assert.False(t, sigs[0].Intact)
	assert.Equal(t, "SIGNATURE_INVALID", DocumentVerdict(sigs, map[string]any{"text_extracted_chars": 10}))
}

func TestContentAppendedAfterSigningIsDetected(t *testing.T) {
	requireOpenSSL(t)
	pdf := sign(t, buildPDF(t, "Bonus 1:1 28-Oct-2024", true), t.TempDir())
	pdf = append(pdf, []byte("\n% unsigned incremental update\n")...)
	sigs, err := VerifyPDFSignatures(pdf)
	require.NoError(t, err)
	assert.True(t, sigs[0].Intact, "the signed ranges are untouched")
	assert.False(t, sigs[0].CoversDocument, "but the document now contains bytes the signature does not cover")
}

func TestUnsignedFilingAndWrongContent(t *testing.T) {
	requireOpenSSL(t)
	unsigned := buildPDF(t, "Bonus 1:1 record date 28-Oct-2024", false)
	sigs, err := VerifyPDFSignatures(unsigned)
	require.NoError(t, err)
	assert.Empty(t, sigs)
	assert.Equal(t, "UNSIGNED_PENDING_REVIEW", DocumentVerdict(sigs, ContentChecks(ExtractText(unsigned), bonus)))

	wrong := sign(t, buildPDF(t, "Bonus 2:1 record date 15-Nov-2024", true), t.TempDir())
	sigs, _ = VerifyPDFSignatures(wrong)
	checks := ContentChecks(ExtractText(wrong), bonus)
	assert.Equal(t, false, checks["ratio_found"])
	assert.Equal(t, "SIGNATURE_INTACT_CONTENT_UNCONFIRMED_PENDING_REVIEW", DocumentVerdict(sigs, checks),
		"an intact signature on a document about something else proves nothing about this action")
}
