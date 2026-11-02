> Source code, full documentation and issues: https://github.com/AceRodnel14/docuscrypt

# DocusCrypt

A small self-hosted web app that encrypts and decrypts password-protected documents: modern
Office files (`.docx`, `.xlsx`, `.pptx`, AES-256 Agile) and PDFs (AES-256 R6). Upload a file,
enter or generate a password, download the result. Files are processed in memory per request;
nothing is stored. Passwords support only standard US-English keyboard letters, digits and symbols,
with no spaces, for now (generated passwords always qualify).

## Quick start
```yaml
services:
  app:
    image: eacer/docuscrypt:latest
    restart: unless-stopped
    ports: ["8080:8080"]
```
`docker compose up -d`, then open `http://<host>:8080`. No `.env`, volume or database is needed.
For HTTPS, put a reverse proxy in front; the app has no TLS or login of its own (meant for a
trusted network).

Upgrading from an older release: the container port changed from `8000` to `8080`; map it with
`-p 8000:8080` to keep the old host port.

Or with a single `docker run`:
```bash
docker run --rm -p 8080:8080 eacer/docuscrypt:latest
```

## Configuration
| Variable | Default | Description |
| :- | :- | :- |
| `PORT` | `8080` | HTTP port inside the container |

Port `8080` (HTTP). No volume: the app is stateless. Health: `/healthz` (liveness), `/readyz`
(readiness); the image's `HEALTHCHECK` uses `/docuscrypt healthcheck`.

## Tags
`latest` (default, latest stable), `main` (default branch), `vX.Y`, `vX.Y.Z` (releases). Platforms: `linux/amd64`, `linux/arm64`.
