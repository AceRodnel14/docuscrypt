package docs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/mgilbir/spine/common/crypto"
	"github.com/mgilbir/spine/opc"
	"github.com/richardlehane/mscfb"
)

// Encrypted OOXML documents are not ZIPs: they're OLE Compound File (CFB)
// containers holding two streams, EncryptionInfo and EncryptedPackage
// ([MS-OFFCRYPTO] §2.3.4). mscfb reads the container; spine does the crypto.
const (
	streamEncryptionInfo   = "EncryptionInfo"
	streamEncryptedPackage = "EncryptedPackage"
)

// readEncryptedStreams pulls the two encryption streams out of a CFB container.
// ok is false when the container is valid but isn't an encrypted OOXML package
// (for example, a legacy .doc/.xls/.ppt renamed to .docx).
func readEncryptedStreams(data []byte) (info, pkg []byte, ok bool, err error) {
	doc, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return nil, nil, false, unsupported("Could not read this as an Office file: %v", err)
	}
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		if entry.Name != streamEncryptionInfo && entry.Name != streamEncryptedPackage {
			continue
		}
		buf := make([]byte, entry.Size)
		if _, err := io.ReadFull(entry, buf); err != nil {
			return nil, nil, false, unsupported("Could not read this as an Office file: %v", err)
		}
		if entry.Name == streamEncryptionInfo {
			info = buf
		} else {
			pkg = buf
		}
	}
	return info, pkg, info != nil && pkg != nil, nil
}

func officeIsEncrypted(data []byte) (bool, error) {
	if bytes.HasPrefix(data, magicZIP) {
		if err := checkOOXMLZip(data); err != nil {
			return false, err
		}
		return false, nil
	}
	_, _, ok, err := readEncryptedStreams(data)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, unsupported("This looks like a legacy Office file (.doc/.xls/.ppt) — only modern formats are supported. Open it in Office and save it as .docx/.xlsx/.pptx first.")
	}
	return true, nil
}

// checkOOXMLZip makes sure a ZIP is actually an Office Open XML package
// rather than some other archive with an Office extension.
func checkOOXMLZip(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return unsupported("Could not read this as an Office file: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			return nil
		}
	}
	return unsupported("Could not read this as an Office file — it's a ZIP archive but not an Office document.")
}

func officeDecrypt(data []byte, password string) ([]byte, error) {
	info, pkg, ok, err := readEncryptedStreams(data)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, unsupported("This file isn't password-protected.")
	}
	plain, err := crypto.Decrypt(info, pkg, password)
	switch {
	case err == nil:
		return plain, nil
	case errors.Is(err, crypto.ErrWrongPassword):
		return nil, &ErrWrongPassword{Msg: "Incorrect password."}
	case errors.Is(err, crypto.ErrIntegrityCheckFailed):
		return nil, &ErrWrongPassword{Msg: "The password was accepted, but the file failed its integrity check — it may be corrupted or tampered with."}
	case errors.Is(err, crypto.ErrUnsupportedEncryption), errors.Is(err, crypto.ErrMalformedEncryptionInfo):
		return nil, unsupported("This file uses an encryption scheme that isn't supported.")
	}
	return nil, fmt.Errorf("office decrypt: %w", err)
}

func officeEncrypt(data []byte, password string) ([]byte, error) {
	if !bytes.HasPrefix(data, magicZIP) {
		if _, _, ok, _ := readEncryptedStreams(data); ok {
			return nil, unsupported("This file is already password-protected — decrypt it first.")
		}
		return nil, unsupported("Could not read this as an Office file.")
	}
	if err := checkOOXMLZip(data); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	// Agile encryption: AES-256-CBC, SHA-512 key derivation, HMAC integrity —
	// the same scheme Office 2010+ writes by default.
	if err := opc.SaveEncrypted(&out, data, password); err != nil {
		if errors.Is(err, crypto.ErrInvalidPassword) {
			return nil, unsupported("Office passwords must be at most 255 characters.")
		}
		return nil, fmt.Errorf("office encrypt: %w", err)
	}
	return out.Bytes(), nil
}
