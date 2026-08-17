package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyForwardsAuthenticatedRequests(t *testing.T) {
	const (
		demoKey = "tvly-dev-demo-key"
		realKey = "tvly-real-upstream-key"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+realKey {
			t.Errorf("Authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if got := string(body); !strings.Contains(got, `"api_key":"`+realKey+`"`) || !strings.Contains(got, `"query":"test"`) {
			t.Errorf("body = %s", got)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()

	proxy, err := NewTavilyProxy(&Config{Upstream: upstream.URL, APIKeys: []string{realKey}, Auth: []string{demoKey}})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		body   string
		header bool
	}{
		{name: "authorization header", body: `{"query":"test"}`, header: true},
		{name: "json body", body: `{"api_key":"` + demoKey + `","query":"test"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(tc.body))
			if tc.header {
				req.Header.Set("Authorization", "Bearer "+demoKey)
			}
			res := httptest.NewRecorder()
			proxy.ServeHTTP(res, req)
			if res.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestProxyRejectsUnreadableBody(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer upstream.Close()

	proxy, err := NewTavilyProxy(&Config{Upstream: upstream.URL, APIKeys: []string{"tvly-real-upstream-key"}, Auth: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/search", nil)
	req.Header.Set("Authorization", "Bearer demo")
	req.Body = errorReader{}
	res := httptest.NewRecorder()

	proxy.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	if called {
		t.Fatal("upstream was called")
	}
}

func TestNewTavilyProxyRejectsInvalidUpstream(t *testing.T) {
	for _, upstream := range []string{"api.tavily.com", "ftp://api.tavily.com"} {
		t.Run(upstream, func(t *testing.T) {
			if _, err := NewTavilyProxy(&Config{Upstream: upstream}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestProxyErrors(t *testing.T) {
	proxy, err := NewTavilyProxy(&Config{Upstream: "http://127.0.0.1:1", APIKeys: []string{"tvly-real-upstream-key"}, Auth: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unauthorized", func(t *testing.T) {
		res := httptest.NewRecorder()
		proxy.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"test"}`)))
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", res.Code)
		}
	})

	t.Run("upstream unavailable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"test"}`))
		req.Header.Set("Authorization", "Bearer demo")
		res := httptest.NewRecorder()
		proxy.ServeHTTP(res, req)
		if res.Code != http.StatusBadGateway {
			t.Fatalf("status = %d", res.Code)
		}
	})
}

func TestLoadConfigAndRotator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("api_keys: [first, second]\nauth: [demo]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:8787" || cfg.Upstream != "https://api.tavily.com" {
		t.Fatalf("defaults = %#v", cfg)
	}
	r := KeyRotator{keys: cfg.APIKeys}
	if r.Len() != 2 || r.Next() != "first" || r.Next() != "second" || r.Next() != "first" {
		t.Fatal("round robin failed")
	}
	if (&KeyRotator{}).Next() != "" {
		t.Fatal("empty rotator returned a key")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "invalid yaml", body: "["},
		{name: "missing api keys", body: "auth: [demo]\n"},
		{name: "missing auth", body: "api_keys: [real]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := LoadConfig(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("expected read error")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errorReader) Close() error             { return nil }
