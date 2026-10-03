package docs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// minimalOOXML builds a tiny but structurally valid .docx package.
func minimalOOXML(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>hello docuscrypt</w:t></w:r></w:p></w:body></w:document>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// minimalPDF builds a one-page PDF with a correct xref table.
func minimalPDF() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"", // content stream, filled below
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	content := "BT /F1 12 Tf 20 100 Td (hello docuscrypt) Tj ET"
	objs[3] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)

	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return []byte(b.String())
}

func TestDetectRouting(t *testing.T) {
	docx, pdf := minimalOOXML(t), minimalPDF()
	cases := []struct {
		name    string
		data    []byte
		want    Kind
		wantErr string
	}{
		{"report.docx", docx, KindOffice, ""},
		{"REPORT.XLSX", docx, KindOffice, ""}, // extension match is case-insensitive
		{"scan.pdf", pdf, KindPDF, ""},
		{"scan.pdf", docx, KindUnknown, "contents look like an Office file"},
		{"report.docx", pdf, KindUnknown, "contents look like a PDF file"},
		{"notes.txt", []byte("hi"), KindUnknown, "not a supported file type"},
		{"old.doc", docx, KindUnknown, "Legacy Office files"},
		{"empty.pdf", nil, KindUnknown, "Could not read this as a PDF"},
	}
	for _, c := range cases {
		got, err := Detect(c.name, c.data, ModeDecrypt)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("%s: want error containing %q, got %v", c.name, c.wantErr, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		kind Kind
		data []byte
	}{
		{KindOffice, minimalOOXML(t)},
		{KindPDF, minimalPDF()},
	} {
		t.Run(tc.kind.String(), func(t *testing.T) {
			const pw = "Passw0rd!"

			if enc, err := IsEncrypted(tc.kind, tc.data); err != nil || enc {
				t.Fatalf("plain file: IsEncrypted = %v, %v", enc, err)
			}

			encrypted, err := Encrypt(tc.kind, tc.data, pw)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if enc, err := IsEncrypted(tc.kind, encrypted); err != nil || !enc {
				t.Fatalf("encrypted file: IsEncrypted = %v, %v", enc, err)
			}

			// Encrypting twice is refused with a friendly message.
			if _, err := Encrypt(tc.kind, encrypted, pw); !IsUserError(err) {
				t.Errorf("re-encrypt: want user error, got %v", err)
			}

			// Wrong password.
			_, err = Decrypt(tc.kind, encrypted, "wrong")
			var wp *ErrWrongPassword
			if !errors.As(err, &wp) {
				t.Fatalf("wrong password: want ErrWrongPassword, got %v", err)
			}

			plain, err := Decrypt(tc.kind, encrypted, pw)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if enc, err := IsEncrypted(tc.kind, plain); err != nil || enc {
				t.Fatalf("decrypted file: IsEncrypted = %v, %v", enc, err)
			}
			// Office decryption restores the exact original package bytes.
			if tc.kind == KindOffice && !bytes.Equal(plain, tc.data) {
				t.Errorf("decrypted Office package differs from the original")
			}
		})
	}
}
