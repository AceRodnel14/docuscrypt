# DocusCrypt

A simple web app for encrypting and decrypting password-protected MS Office files
(`.doc`, `.docx`, `.xls`, `.xlsx`, `.ppt`, `.pptx`), built on top of
[msoffcrypto-tool](https://pypi.org/project/msoffcrypto-tool/).

- Upload a file, toggle Encrypt/Decrypt, provide a password (or generate one).
- Files are processed in a per-request temp directory on the container's own
  ephemeral filesystem — nothing is written to a volume, and everything is
  deleted immediately after the response is sent, whether it succeeds or fails.
- Stateless: each request is fully self-contained, so there's no session/cookie
  tracking and no risk of one user's file ever crossing paths with another's,
  even under concurrent use.

## Format support

| Format | Decrypt | Encrypt |
|---|---|---|
| `.docx` / `.xlsx` / `.pptx` (2007+) | ✅ | ✅ (experimental — see note below) |
| `.doc` / `.xls` / `.ppt` (97–2003) | ✅ | ❌ not supported |

Encryption is only possible for modern (OOXML) formats — this is a limitation
of the underlying library, not this app. It's also flagged upstream as an
experimental feature, so treat encrypted output as "should work" rather than
"guaranteed."

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

A `/healthz` endpoint is available for liveness/readiness probes.

## Usage guide

1. **Choose a mode** — click **Decrypt** or **Encrypt** at the top of the page.
2. **Pick a file** — click the upload box or drag a file onto it. The app
   checks the file extension against the supported list for whichever mode
   you've selected, and for decryption, also verifies the file is actually
   password-protected before you spend a password guess on it.
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

## Project layout

```
docuscrypt/
├── app/
│   ├── main.py           # FastAPI routes
│   ├── crypto.py         # msoffcrypto-tool wrapper (encrypt/decrypt/format checks)
│   ├── passwords.py       # secrets-based password generator
│   ├── templates/
│   │   └── index.html
│   └── static/
│       ├── style.css
│       └── script.js
├── requirements.txt
├── Dockerfile
├── .dockerignore
└── README.md
```

## Next: Kubernetes

This is designed to deploy cleanly with the homelab's existing
Traefik + KEDA scale-to-zero template, minus the PVC (this app deliberately
has no persistent storage needs). That manifest set is a separate follow-up.
