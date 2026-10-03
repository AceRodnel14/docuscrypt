// Package docs routes each uploaded document to the right encryption backend.
//
// Routing is content-first: the file's leading bytes decide which tool handles
// it, and the extension only has to agree with what the bytes say.
//
//   - PDF (.pdf)                          -> pdfcpu  (AES-256)
//   - OOXML Office (.docx/.xlsx/.pptx)    -> spine   (ECMA-376 Agile, AES-256/SHA-512)
//
// Legacy binary Office formats (.doc/.xls/.ppt) are recognised and rejected
// with a clear message: no pure-Go library implements their RC4 encryption.
package docs

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Kind identifies which backend handles a document.
type Kind int

const (
	KindUnknown Kind = iota
	KindOffice       // OOXML package: plain ZIP, or an encrypted OLE/CFB container
	KindPDF
)

func (k Kind) String() string {
	switch k {
	case KindOffice:
		return "Office"
	case KindPDF:
		return "PDF"
	}
	return "unknown"
}

// Mode is the requested operation.
type Mode string

const (
	ModeEncrypt Mode = "encrypt"
	ModeDecrypt Mode = "decrypt"
)

// ErrUnsupportedFile means the file's type or format can't be used for the
// requested operation. Its message is safe to show to the user.
type ErrUnsupportedFile struct{ Msg string }

func (e *ErrUnsupportedFile) Error() string { return e.Msg }

func unsupported(format string, a ...any) error {
	return &ErrUnsupportedFile{Msg: fmt.Sprintf(format, a...)}
}

// ErrWrongPassword means decryption failed because of the password (or, for
// files without an integrity check, possibly corruption). Its message is safe
// to show to the user.
type ErrWrongPassword struct{ Msg string }

func (e *ErrWrongPassword) Error() string { return e.Msg }

// IsUserError reports whether err carries a message meant for the end user
// (wrong password, unsupported file) rather than an internal failure.
func IsUserError(err error) bool {
	var u *ErrUnsupportedFile
	var w *ErrWrongPassword
	return errors.As(err, &u) || errors.As(err, &w)
}

var extKinds = map[string]Kind{
	".docx": KindOffice,
	".xlsx": KindOffice,
	".pptx": KindOffice,
	".pdf":  KindPDF,
}

var legacyOfficeExts = map[string]string{".doc": ".docx", ".xls": ".xlsx", ".ppt": ".pptx"}

// SupportedExtensions lists the extensions usable for mode, sorted.
// Every supported type can currently be both encrypted and decrypted.
func SupportedExtensions(Mode) []string {
	exts := make([]string, 0, len(extKinds))
	for e := range extKinds {
		exts = append(exts, e)
	}
	slices.Sort(exts)
	return exts
}

// Ext returns the lower-cased extension of filename, including the dot.
func Ext(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}

// CheckExtension is the fast, upfront check based on the file name alone.
func CheckExtension(filename string, mode Mode) (Kind, error) {
	ext := Ext(filename)
	if k, ok := extKinds[ext]; ok {
		return k, nil
	}
	if modern, ok := legacyOfficeExts[ext]; ok {
		return KindUnknown, unsupported(
			"Legacy Office files ('%s') aren't supported — only modern formats can be %sed. "+
				"Open it in Office and save it as '%s' first.", ext, mode, modern)
	}
	if ext == "" {
		ext = "unknown"
	}
	return KindUnknown, unsupported("'%s' is not a supported file type for %s. Supported: %s.",
		ext, mode, strings.Join(SupportedExtensions(mode), ", "))
}

var (
	magicZIP = []byte("PK\x03\x04")
	magicCFB = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	magicPDF = []byte("%PDF-")
)

// sniff identifies a document by its leading bytes.
func sniff(data []byte) Kind {
	switch {
	case bytes.HasPrefix(data, magicZIP), bytes.HasPrefix(data, magicCFB):
		return KindOffice
	// The PDF spec allows junk before the header; readers accept it within 1 KiB.
	case bytes.Contains(data[:min(len(data), 1024)], magicPDF):
		return KindPDF
	}
	return KindUnknown
}

// Detect checks the file name and contents together and returns the backend
// that should handle it. The extension must agree with what the bytes say.
func Detect(filename string, data []byte, mode Mode) (Kind, error) {
	byExt, err := CheckExtension(filename, mode)
	if err != nil {
		return KindUnknown, err
	}
	byContent := sniff(data)
	switch {
	case byContent == KindUnknown:
		return KindUnknown, unsupported("Could not read this as %s file — its contents don't match the '%s' extension.",
			article(byExt), Ext(filename))
	case byContent != byExt:
		return KindUnknown, unsupported("This file is named '%s' but its contents look like %s file. Rename it with the right extension and try again.",
			Ext(filename), article(byContent))
	}
	return byExt, nil
}

func article(k Kind) string {
	if k == KindOffice {
		return "an Office"
	}
	return "a " + k.String()
}

// IsEncrypted reports whether data is password-protected. It also acts as a
// deeper validity check than the extension: unreadable files return
// ErrUnsupportedFile.
func IsEncrypted(kind Kind, data []byte) (bool, error) {
	switch kind {
	case KindOffice:
		return officeIsEncrypted(data)
	case KindPDF:
		return pdfIsEncrypted(data)
	}
	return false, unsupported("Unsupported file type.")
}

// Decrypt removes password protection, returning the plain document.
func Decrypt(kind Kind, data []byte, password string) ([]byte, error) {
	switch kind {
	case KindOffice:
		return officeDecrypt(data, password)
	case KindPDF:
		return pdfDecrypt(data, password)
	}
	return nil, unsupported("Unsupported file type.")
}

// Encrypt password-protects a plain document.
func Encrypt(kind Kind, data []byte, password string) ([]byte, error) {
	switch kind {
	case KindOffice:
		return officeEncrypt(data, password)
	case KindPDF:
		return pdfEncrypt(data, password)
	}
	return nil, unsupported("Unsupported file type.")
}
