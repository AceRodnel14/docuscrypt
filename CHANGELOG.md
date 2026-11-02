# Changelog

## 3.0.0

Major release: a lot changed at once, some of it breaking for existing setups.

- Breaking: the container port is `8080` (was `8000`), set with the new `PORT` variable. Keep the old host port with
  `-p 8000:8080` (Compose: `APP_PORT=8000`) and update any proxy target.
- Breaking: the default image is `eacer/docuscrypt:latest`; pin a release with `IMAGE=eacer/docuscrypt:vX.Y` /
  `vX.Y.Z`. Releases are tagged `vX.Y` and `vX.Y.Z` from now on (the older `v2.YYYYMMDD` tags end here).
- Self-hosting with Docker Compose: new `docker-compose.yml` (stateless, no volume), `.env.example` (optional),
  README **Self-hosting** section and `dockerdesc.md` for Docker Hub.
- New logo: an SVG favicon for the browser tab.
- Health: new `/readyz`, and a `healthcheck` subcommand used by the image's `HEALTHCHECK` (distroless has no curl).
- CI uses the shared gitlab-actions `app.yml` pipeline (lint + tests on merge requests, test build with smoke run,
  multi-arch image, GitHub sync); `.golangci.yml` from the app template.
- Build image pinned to its minor version (`golang:1.26-alpine3.24`); the distroless runtime keeps its rolling tag.
- The version is built into the binary (`main.version`, Dockerfile `ARG VERSION`) and logged at startup.
- Known limitation: passwords are limited to the letters, digits and symbols of a standard US-English keyboard, with
  no spaces; other characters are not supported yet and may make a PDF undecryptable (see README **Usage guide**).
