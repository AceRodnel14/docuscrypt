package docs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pdfConfig returns a stateless pdfcpu configuration (no config dir on disk,
// which matters for a non-root container with no home directory).
//
// DocusCrypt exposes a single password, so it is used as both the user
// (open) password and the owner (permissions) password: whoever knows it
// gets the whole document, with nothing restricted.
func pdfConfig(password string) *model.Configuration {
	conf := model.NewAESConfiguration(password, password, 256)
	conf.Permissions = model.PermissionsAll
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

func isWrongPDFPassword(err error) bool {
	return errors.Is(err, pdfcpu.ErrWrongPassword) ||
		strings.Contains(err.Error(), pdfcpu.ErrWrongPassword.Error())
}

func pdfIsEncrypted(data []byte) (bool, error) {
	conf := pdfConfig("")
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(data), conf)
	if err != nil {
		if isWrongPDFPassword(err) {
			return true, nil // needs a password just to open
		}
		return false, unsupported("Could not read this as a PDF file: %v", err)
	}
	// Opened without a password but still carries an Encrypt dictionary:
	// an owner-password-only (permissions-restricted) PDF.
	return ctx.E != nil, nil
}

// latin1Password re-encodes a password the way the older PDF schemes (RC4 and
// AES-128, revisions 2–4) expect: one byte per character, Latin-1 /
// PDFDocEncoding, which is what Acrobat and pypdf write. pdfcpu passes the raw
// UTF-8 bytes instead, so a non-ASCII password like "Pässwort" wouldn't match.
// ok is false when there's nothing different to try.
func latin1Password(pw string) (string, bool) {
	b := make([]byte, 0, len(pw))
	changed := false
	for _, r := range pw {
		if r > 0xFF {
			return "", false
		}
		if r > 0x7F {
			changed = true
		}
		b = append(b, byte(r))
	}
	return string(b), changed
}

func pdfDecrypt(data []byte, password string) ([]byte, error) {
	candidates := []string{password}
	if alt, ok := latin1Password(password); ok {
		candidates = append(candidates, alt)
	}
	for _, pw := range candidates {
		var out bytes.Buffer
		err := api.Decrypt(context.Background(), bytes.NewReader(data), &out, pdfConfig(pw))
		if err == nil {
			return out.Bytes(), nil
		}
		if !isWrongPDFPassword(err) {
			return nil, fmt.Errorf("pdf decrypt: %w", err)
		}
	}
	return nil, &ErrWrongPassword{Msg: "Incorrect password."}
}

func pdfEncrypt(data []byte, password string) ([]byte, error) {
	encrypted, err := pdfIsEncrypted(data)
	if err != nil {
		return nil, err
	}
	if encrypted {
		return nil, unsupported("This PDF is already password-protected — decrypt it first.")
	}

	// This is api.Encrypt unrolled so the document can be marked PDF 2.0
	// before writing: pdfcpu only emits AES-256 revision 6 (ISO 32000-2, with
	// iterated password hashing) for PDF 2.0 files, and otherwise falls back to
	// the deprecated revision 5, whose single-SHA-256 check is far quicker to
	// brute-force. Every current reader opens R6.
	conf := pdfConfig(password)
	conf.Cmd = model.ENCRYPT
	ctx, err := api.ReadValidateAndOptimize(context.Background(), bytes.NewReader(data), conf, nil)
	if err != nil {
		return nil, unsupported("Could not read this as a PDF file: %v", err)
	}
	v20 := model.V20
	ctx.HeaderVersion = &v20
	if ctx.RootVersion != nil {
		ctx.RootVersion = &v20
	}

	var out bytes.Buffer
	if err := api.WriteContext(context.Background(), ctx, &out); err != nil {
		return nil, fmt.Errorf("pdf encrypt: %w", err)
	}
	return out.Bytes(), nil
}
