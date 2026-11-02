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

## Self-hosting

### Requirements
- Docker with the Compose plugin, on `linux/amd64` or `linux/arm64`.
- Optional: a reverse proxy in front for HTTPS. The app itself only serves plain HTTP on
  port `8080` and has no TLS or login of its own by design (meant for a trusted network).

### Quick start
> **No `.env` is needed**: docuscrypt runs with the defaults. To publish on another host port
> (`APP_PORT`) or pin a release (`IMAGE`), create `.env` from `.env.example` before the first
> start. Keep `PORT=8080` under Compose.

```bash
mkdir docuscrypt && cd docuscrypt
curl -fsSLO https://raw.githubusercontent.com/AceRodnel14/docuscrypt/main/docker-compose.yml
docker compose pull
docker compose up -d
```
Open `http://<host>:8080`. Check health with `curl -f http://<host>:8080/healthz`.
To build from source instead, see [Development](#development).

> **Upgrading from an older release:** the container port changed from `8000` to `8080`.
> Update your port mapping (`-p 8000:8080` keeps the old host port; with Compose set
> `APP_PORT=8000`) and any proxy target pointing at the container's port.

### Try it with `docker run`
```bash
docker run --rm -p 8080:8080 eacer/docuscrypt:latest
```

### Configuration
| Variable | Default | Description |
| :- | :- | :- |
| `PORT` | `8080` | HTTP port inside the container |
| `IMAGE` | `eacer/docuscrypt:latest` | Image (and tag) Compose runs or builds |
| `APP_BIND` | `0.0.0.0` | Host address Compose publishes on (`127.0.0.1` behind a local proxy) |
| `APP_PORT` | `8080` | Host port Compose publishes |

### Data and backups
Nothing to back up: there is no volume and no database. Files are processed in memory per
request, and anything that spills to the container's temp dir (very large uploads) is
deleted as soon as the response is sent.

### Upgrading
`latest` is the latest stable image and what Compose uses by default; `main` follows the default
branch. Release tags `vX.Y` and `vX.Y.Z` are pushed when a release is tagged: pin `IMAGE` to `vX.Y`
for patch updates only, then `docker compose pull && docker compose up -d`.

### Health endpoints
- `/healthz`: the process is up (liveness).
- `/readyz`: ready to serve (readiness). docuscrypt has no dependencies, so it answers `200`
  as soon as the server is up.
- The image's `HEALTHCHECK` runs `/docuscrypt healthcheck` (the runtime image is distroless,
  with no shell or curl). The server drains in-flight requests on `SIGTERM`, which plays
  nicely with KEDA scale-to-zero.

## Development
Requires Go 1.26+ (or only Docker).

```bash
docker compose up --build   # the whole app, as self-hosters run it
go test ./...               # unit tests (routing, Office + PDF round-trips, passwords, HTTP)
go run .                    # serves on http://localhost:8080 (PORT to change)
```
`./manual_cicd_lint.sh` runs the CI lint/test jobs locally in throwaway containers (`--list`, `--only`, `--skip`).

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
   - **Password characters (limitation):** use only the letters, digits and
     symbols on a standard US-English keyboard, with no spaces (e.g.
     `Correct-Horse_9!`). Spaces, accented or non-Latin letters, emoji and
     symbols like `€` are not supported yet, for any file type: DocusCrypt
     aims to work on the most basic setup, a machine without extra fonts.
     Encryption doesn't block them, so a file (PDFs in particular) encrypted
     with such a password may not decrypt in DocusCrypt. Passwords from
     **Generate password**, with or without symbols, always stay within the
     supported set.
4. **Submit** — click **Decrypt file** / **Encrypt file**. On success, the
   processed file downloads automatically (named `<original>_decrypted.ext`
   or `<original>_encrypted.ext`). On failure — wrong password, unsupported
   format, or anything else — a simple OK-only dialog explains what went
   wrong; nothing partially downloads or lingers on the server either way.

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/` | Web UI |
| `GET` | `/healthz` | Liveness: `{"status":"ok"}` |
| `GET` | `/readyz` | Readiness: `200` once the server is up |
| `GET` | `/api/generate-password?length=16&symbols=false` | Random password (length clamped to 4–128) |
| `POST` | `/api/check` | Pre-flight: form fields `mode`, `file` → `{"ok": true}` or `{"ok": false, "error": "..."}` |
| `POST` | `/api/process` | Form fields `mode`, `password`, `file` → processed file, or `{"detail": "..."}` on error |

## Project layout

```
docuscrypt/
├── main.go                 # HTTP server, routes, healthcheck subcommand (embeds app/)
├── main_test.go            # HTTP tests (/healthz, /readyz)
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
├── docker-compose.yml
├── .env.example
├── .dockerignore
├── .gitlab-ci.yml          # gitlab-actions app.yml pipeline
├── .golangci.yml
├── dockerdesc.md           # Docker Hub description
├── CHANGELOG.md
├── LICENSE
└── README.md
```

## Next: Kubernetes

This is designed to deploy cleanly with the homelab's existing
Traefik + KEDA scale-to-zero template, minus the PVC (this app deliberately
has no persistent storage needs). That manifest set is a separate follow-up.

---
_Developed with assistance from Claude (Claude Opus 5.5, Claude Sonnet 5.5)._
