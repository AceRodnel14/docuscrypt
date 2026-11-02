# --- build ---
FROM golang:1.26-alpine3.24 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static binary; the HTML template and static assets are embedded into it.
ARG VERSION=3.0.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /docuscrypt .

# --- runtime ---
# Distroless: no shell or package manager, just the binary. Runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /docuscrypt /docuscrypt
USER nonroot:nonroot

ENV PORT=8080
EXPOSE 8080
# `docuscrypt healthcheck` GETs http://127.0.0.1:$PORT/healthz and exits non-zero on failure
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/docuscrypt", "healthcheck"]
ENTRYPOINT ["/docuscrypt"]
