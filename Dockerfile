# --- build ---
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static binary; the HTML template and static assets are embedded into it.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /docuscrypt .

# --- runtime ---
# Distroless: no shell or package manager, just the binary. Runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /docuscrypt /docuscrypt
USER nonroot:nonroot

EXPOSE 8000

ENTRYPOINT ["/docuscrypt"]
