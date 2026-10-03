# DocusCrypt

A simple web app for encrypting and decrypting password-protected documents:
modern Office files (`.docx`, `.xlsx`, `.pptx`) and PDFs. Written in Go as a
single static binary with the web UI embedded.

- Upload a file, toggle Encrypt/Decrypt, provide a password (or generate one).
- DocusCrypt picks the right engine for each file automatically, based on what
  the file actually contains (not just its name).
- Files are processed in memory, per request. Nothing is written to a volume,
  and anything that spills to the container's temp dir (very large uploads) is
  deleted as soon as the response is sent, whether it succeeds or fails.
- Stateless: each request is fully self-contained, so there's no session/cookie
  tracking and no risk of one user's file ever crossing paths with another's,
  even under concurrent use.

## Format support

| Format | Decrypt | Encrypt | Engine |
|---|---|---|---|
| `.docx` / `.xlsx` / `.pptx` (2007+) | ✅ | ✅ AES-256 (Agile) | [spine](https://github.com/mgilbir/spine) |
| `.pdf` | ✅ | ✅ AES-256 (R6) | [pdfcpu](https://github.com/pdfcpu/pdfcpu) |
| `.doc` / `.xls` / `.ppt` (97–2003) | ❌ | ❌ | — |

**How a file is routed.** The first bytes of the upload decide the engine: a
PDF header goes to pdfcpu; a ZIP (plain Office) or OLE container (encrypted
Office) goes to spine. The extension must agree with the contents, so a PDF
renamed to `.docx` gets a clear "this looks like a PDF" message instead of a
cryptic failure.

**Office.** Encryption uses the same scheme Office 2010+ writes by default
(ECMA-376 Agile: AES-256, SHA-512 key derivation, HMAC integrity check).
Decryption also accepts the older Office 2007 "Standard" scheme and RC4
CryptoAPI. Files encrypted by DocusCrypt v1 (msoffcrypto-tool) decrypt fine.
For Agile files the integrity check is enforced, so a tampered file is
rejected rather than silently decrypted.

**PDF.** Encryption writes AES-256 revision 6 (ISO 32000-2), which marks the
output as PDF 2.0. All current readers open it. The one password you enter is
used as both the open password and the permissions password, so whoever knows
it gets the whole document with nothing restricted. Decryption accepts AES-256,
AES-128 and RC4 PDFs, including owner-password-only (permissions-restricted)
ones.

**Legacy Office (`.doc`/`.xls`/`.ppt`).** Not supported: no Go library
implements their encryption. DocusCrypt v1 (Python) could *decrypt* these; if
you still need that, keep a v1 image around.

## Running with Docker

Build the image from the project root:

```bash
docker build -t docuscrypt .
```

Run it:

```bash
docker run --rm -p 8000:8000 docuscrypt
```

Then open **http://localhost:8000** in a browser.

No environment variables or volumes are required — the container is fully
self-contained and stateless. If you want to put it behind Traefik (or any
other reverse proxy) for HTTPS, just point the proxy at this container's port
`8000`; the app itself has no TLS/auth logic of its own by design (intended
for a trusted homelab network).

A `/healthz` endpoint is available for liveness/readiness probes. The runtime
image is distroless (no shell), so use HTTP probes rather than `exec` ones.
The server drains in-flight requests on `SIGTERM`, which plays nicely with
KEDA scale-to-zero.

## Running locally without Docker

Requires Go 1.26+.

```bash
go run .          # serves on http://localhost:8000
go test ./...     # unit tests (routing, Office + PDF round-trips, passwords)
```

## Usage guide

1. **Choose a mode** — click **Decrypt** or **Encrypt** at the top of the page.
2. **Pick a file** — click the upload box or drag a file onto it. The app
   checks the file type for whichever mode you've selected, and for
   decryption, also verifies the file is actually password-protected before
   you spend a password guess on it.
3. **Set a password**:
   - Type or paste one directly, or
   - (Encrypt mode only) use the **Generate password** control — adjust the
     length slider and optionally include symbols, then click **Generate**.
   - Click the eye icon at any time to reveal/hide the password field, whether
     it was typed, pasted, or generated.
4. **Submit** — click **Decrypt file** / **Encrypt file**. On success, the
   processed file downloads automatically (named `<original>_decrypted.ext`
   or `<original>_encrypted.ext`). On failure — wrong password, unsupported
   format, or anything else — a simple OK-only dialog explains what went
   wrong; nothing partially downloads or lingers on the server either way.

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/` | Web UI |
| `GET` | `/healthz` | Liveness/readiness: `{"status":"ok"}` |
| `GET` | `/api/generate-password?length=16&symbols=false` | Random password (length clamped to 4–128) |
| `POST` | `/api/check` | Pre-flight: form fields `mode`, `file` → `{"ok": true}` or `{"ok": false, "error": "..."}` |
| `POST` | `/api/process` | Form fields `mode`, `password`, `file` → processed file, or `{"detail": "..."}` on error |

## Project layout

```
docuscrypt/
├── main.go                 # HTTP server and routes (embeds app/)
├── internal/
│   ├── docs/
│   │   ├── docs.go         # file-type detection and routing
│   │   ├── office.go       # Office encrypt/decrypt (spine + mscfb)
│   │   └── pdf.go          # PDF encrypt/decrypt (pdfcpu)
│   └── passwords/
│       └── passwords.go    # crypto/rand password generator
├── app/
│   ├── templates/
│   │   └── index.html
│   └── static/
│       ├── style.css
│       └── script.js
├── go.mod / go.sum
├── Dockerfile
├── .dockerignore
└── README.md
```

## Next: Kubernetes

This is designed to deploy cleanly with the homelab's existing
Traefik + KEDA scale-to-zero template, minus the PVC (this app deliberately
has no persistent storage needs). That manifest set is a separate follow-up.
