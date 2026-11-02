package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbes(t *testing.T) {
	h, err := routes()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200", p, w.Code)
		}
	}
}

func TestFavicon(t *testing.T) {
	h, err := routes()
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/static/favicon.svg", nil))
	if w.Code != http.StatusOK {
		t.Errorf("favicon = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("favicon Content-Type = %q, want image/svg+xml", ct)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), `rel="icon"`) {
		t.Error(`index page lacks rel="icon" link`)
	}
}

func TestListenPort(t *testing.T) {
	cases := []struct {
		env, want string
		ok        bool
	}{
		{"", "8080", true},
		{"9000", "9000", true},
		{"0", "", false},
		{"70000", "", false},
		{"abc", "", false},
		{":80", "", false},
	}
	for _, c := range cases {
		t.Setenv("PORT", c.env)
		got, err := listenPort()
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("PORT=%q: got %q, %v", c.env, got, err)
		}
	}
}

func TestHealthcheck(t *testing.T) {
	h, err := routes()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	t.Setenv("PORT", srv.Listener.Addr().String()[len("127.0.0.1:"):])
	if got := healthcheck(); got != 0 {
		t.Errorf("healthcheck = %d, want 0", got)
	}
	srv.Close()
	if got := healthcheck(); got != 1 {
		t.Errorf("healthcheck with server down = %d, want 1", got)
	}
}
