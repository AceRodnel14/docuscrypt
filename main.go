// DocusCrypt: a small web app for encrypting and decrypting password-protected
// documents (modern Office files and PDFs).
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AceRodnel14/docuscrypt/internal/docs"
	"github.com/AceRodnel14/docuscrypt/internal/passwords"
)

const (
	listenAddr = ":8000"
	// Uploads up to this size are parsed in memory; anything larger spills to
	// the container's own temp dir and is removed when the request ends.
	multipartMemory = 32 << 20
)

//go:embed app/templates/index.html
var indexHTML string

//go:embed app/static
var staticFiles embed.FS

var (
	logger    = slog.New(slog.NewTextHandler(os.Stdout, nil)).With("logger", "docuscrypt")
	indexTmpl = template.Must(template.New("index").Parse(indexHTML))
)

func main() {
	static, err := fs.Sub(staticFiles, "app/static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/generate-password", handleGeneratePassword)
	mux.HandleFunc("POST /api/check", handleCheck)
	mux.HandleFunc("POST /api/process", handleProcess)

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Drain in-flight requests on SIGTERM (Kubernetes / KEDA scale-down).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logger.Info("listening", "addr", listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // keep generated passwords like "a&b" readable
	_ = enc.Encode(v)
}

// writeDetail mirrors FastAPI's HTTPException body: {"detail": "..."}.
func writeDetail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"detail": msg})
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := indexTmpl.Execute(w, map[string]any{
		"DecryptExts": docs.SupportedExtensions(docs.ModeDecrypt),
		"EncryptExts": docs.SupportedExtensions(docs.ModeEncrypt),
	})
	if err != nil {
		logger.Error("render index", "err", err)
	}
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleGeneratePassword(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	length := 16
	if v := q.Get("length"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeDetail(w, http.StatusUnprocessableEntity, "length must be an integer.")
			return
		}
		length = n
	}

	useSymbols := false
	if v := q.Get("symbols"); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			useSymbols = true
		case "false", "0", "no", "off":
		default:
			writeDetail(w, http.StatusUnprocessableEntity, "symbols must be a boolean.")
			return
		}
	}

	pw, err := passwords.Generate(length, useSymbols)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "Could not generate a password.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": pw})
}

// readUpload parses the multipart form and returns the uploaded file's name
// and bytes. The caller must defer cleanup().
func readUpload(r *http.Request) (name string, data []byte, cleanup func(), err error) {
	cleanup = func() {}
	if err = r.ParseMultipartForm(multipartMemory); err != nil {
		return "", nil, cleanup, err
	}
	cleanup = func() { _ = r.MultipartForm.RemoveAll() }

	f, hdr, err := r.FormFile("file")
	if err != nil {
		return "", nil, cleanup, err
	}
	defer f.Close()

	data, err = io.ReadAll(f)
	return filepath.Base(hdr.Filename), data, cleanup, err
}

func parseMode(s string) (docs.Mode, bool) {
	m := docs.Mode(s)
	return m, m == docs.ModeEncrypt || m == docs.ModeDecrypt
}

// handleCheck is a pre-flight check only: no password needed, it validates the
// file is the right type for the chosen mode (and, for decrypt, is actually
// encrypted).
func handleCheck(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, msg string) {
		writeJSON(w, status, map[string]any{"ok": false, "error": msg})
	}

	name, data, cleanup, err := readUpload(r)
	defer cleanup()
	if err != nil {
		fail(http.StatusBadRequest, "A file upload is required.")
		return
	}
	mode, ok := parseMode(r.FormValue("mode"))
	logger.Info("check start", "mode", mode, "filename", name)
	if !ok {
		fail(http.StatusBadRequest, "Invalid mode.")
		return
	}

	kind, err := docs.Detect(name, data, mode)
	if err != nil {
		logger.Warn("check rejected", "filename", name, "reason", err)
		fail(http.StatusBadRequest, err.Error())
		return
	}

	encrypted, err := docs.IsEncrypted(kind, data)
	if err != nil {
		logger.Warn("check unreadable", "filename", name, "reason", err)
		fail(http.StatusBadRequest, err.Error())
		return
	}
	switch {
	case mode == docs.ModeDecrypt && !encrypted:
		logger.Info("check not encrypted", "filename", name)
		fail(http.StatusBadRequest, "This file isn't password-protected — nothing to decrypt.")
		return
	case mode == docs.ModeEncrypt && encrypted:
		logger.Info("check already encrypted", "filename", name)
		fail(http.StatusBadRequest, "This file is already password-protected — decrypt it first.")
		return
	}

	logger.Info("check ok", "filename", name, "kind", kind)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleProcess(w http.ResponseWriter, r *http.Request) {
	name, data, cleanup, err := readUpload(r)
	defer func() {
		cleanup()
		logger.Info("process cleaned up", "filename", name)
	}()
	if err != nil {
		writeDetail(w, http.StatusBadRequest, "A file upload is required.")
		return
	}

	mode, ok := parseMode(r.FormValue("mode"))
	logger.Info("process start", "mode", mode, "filename", name, "size", len(data))
	if !ok {
		writeDetail(w, http.StatusBadRequest, "mode must be 'encrypt' or 'decrypt'.")
		return
	}
	password := r.FormValue("password")
	if password == "" {
		writeDetail(w, http.StatusBadRequest, "Password is required.")
		return
	}

	kind, err := docs.Detect(name, data, mode)
	if err != nil {
		logger.Warn("process rejected", "filename", name, "reason", err)
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}

	t0 := time.Now()
	var out []byte
	if mode == docs.ModeDecrypt {
		encrypted, cerr := docs.IsEncrypted(kind, data)
		switch {
		case cerr != nil:
			err = cerr
		case !encrypted:
			writeDetail(w, http.StatusBadRequest, "This file isn't password-protected.")
			return
		default:
			out, err = docs.Decrypt(kind, data, password)
		}
	} else {
		out, err = docs.Encrypt(kind, data, password)
	}
	if err != nil {
		if docs.IsUserError(err) {
			logger.Warn("process failed", "filename", name, "reason", err)
			writeDetail(w, http.StatusBadRequest, err.Error())
			return
		}
		logger.Error("process unexpected error", "filename", name, "err", err)
		writeDetail(w, http.StatusInternalServerError, "Unexpected error: "+err.Error())
		return
	}

	ext := filepath.Ext(name)
	suffix := "_encrypted"
	if mode == docs.ModeDecrypt {
		suffix = "_decrypted"
	}
	outName := strings.TrimSuffix(name, ext) + suffix + ext

	logger.Info("process crypto done", "mode", mode, "filename", name, "kind", kind,
		"elapsed", time.Since(t0).Round(time.Millisecond), "out_size", len(out))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", contentDisposition(outName))
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
	logger.Info("process sent response", "filename", outName, "bytes", len(out))
}

// contentDisposition builds an attachment header with a plain ASCII filename
// (what the frontend parses) plus an RFC 5987 UTF-8 name for non-ASCII files.
func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	h := `attachment; filename="` + ascii + `"`
	if ascii != name {
		h += "; filename*=UTF-8''" + url.PathEscape(name)
	}
	return h
}
