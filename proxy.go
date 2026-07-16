package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type TavilyProxy struct {
	rotator  *KeyRotator
	upstream *url.URL
	authKeys map[string]bool
}

func NewTavilyProxy(cfg *Config) (*TavilyProxy, error) {
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("解析上游地址失败: %w", err)
	}

	authMap := make(map[string]bool, len(cfg.Auth))
	for _, k := range cfg.Auth {
		authMap[k] = true
	}

	return &TavilyProxy{
		rotator:  &KeyRotator{keys: cfg.APIKeys},
		upstream: u,
		authKeys: authMap,
	}, nil
}

func (p *TavilyProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 入站认证
	provided := r.Header.Get("Authorization")
	provided = strings.TrimPrefix(provided, "Bearer ")
	if !p.authKeys[provided] {
		log.Printf("[%s] %s %s -> 401 (认证失败)", r.RemoteAddr, r.Method, r.URL.Path)
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	key := p.rotator.Next()
	if key == "" {
		http.Error(w, `{"error": "no api keys configured"}`, http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(p.upstream)

	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = p.upstream.Host
		req.Header.Set("Authorization", "Bearer "+key)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		log.Printf("[%s] %s %s -> %d (key: %s...%s)",
			r.RemoteAddr, r.Method, r.URL.Path,
			resp.StatusCode,
			key[:6], key[len(key)-4:])
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[%s] %s %s -> 代理错误: %v (key: %s...%s)",
			r.RemoteAddr, r.Method, r.URL.Path, err,
			key[:6], key[len(key)-4:])
		http.Error(w, `{"error": "proxy error"}`, http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
