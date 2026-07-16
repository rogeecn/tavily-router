package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type TavilyProxy struct {
	rotator  *KeyRotator
	upstream *url.URL
}

func NewTavilyProxy(cfg *Config) (*TavilyProxy, error) {
	u, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("解析上游地址失败: %w", err)
	}

	return &TavilyProxy{
		rotator:  &KeyRotator{keys: cfg.APIKeys},
		upstream: u,
	}, nil
}

func (p *TavilyProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
