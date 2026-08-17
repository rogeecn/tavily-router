package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("上游地址必须是完整的 HTTP(S) URL: %q", cfg.Upstream)
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
	// 读取 body（可能需要在转发前替换 api_key）
	var bodyBytes []byte
	var bodyMap map[string]interface{}

	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			log.Printf("[%s] %s %s -> 400 (读取请求体失败: %v)", r.RemoteAddr, r.Method, r.URL.Path, err)
			http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
			return
		}
		_ = json.Unmarshal(bodyBytes, &bodyMap) // body 可能为非 JSON，忽略错误
	}

	// 入站认证：同时支持两种方式
	//   1) Authorization: Bearer <token>（标准方式）
	//   2) JSON body 中的 api_key 字段（Hermes Tavily provider 使用的方式）
	authenticated := false

	// 方式 1: Authorization header
	provided := r.Header.Get("Authorization")
	provided = strings.TrimPrefix(provided, "Bearer ")
	if p.authKeys[provided] {
		authenticated = true
	}

	// 方式 2: body api_key 字段
	if !authenticated && bodyMap != nil {
		if apiKey, ok := bodyMap["api_key"].(string); ok {
			if p.authKeys[apiKey] {
				authenticated = true
			}
		}
	}

	if !authenticated {
		log.Printf("[%s] %s %s -> 401 (认证失败)", r.RemoteAddr, r.Method, r.URL.Path)
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	key := p.rotator.Next()
	if key == "" {
		http.Error(w, `{"error": "no api keys configured"}`, http.StatusInternalServerError)
		return
	}

	// 将 body 中的 api_key 替换为真实的轮询 key，确保上游 Tavily 收到正确的 key
	if bodyMap != nil {
		bodyMap["api_key"] = key
		newBody, err := json.Marshal(bodyMap)
		if err != nil {
			log.Printf("[%s] %s %s -> 500 (编码请求体失败: %v)", r.RemoteAddr, r.Method, r.URL.Path, err)
			http.Error(w, `{"error": "request body encoding failed"}`, http.StatusInternalServerError)
			return
		}
		bodyBytes = newBody
	}

	// 还原 body 供反向代理转发
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))

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
		log.Printf("[%s] %s %s -> 代理错误: %v (key: %s...%s)", r.RemoteAddr, r.Method, r.URL.Path, err,
			key[:6], key[len(key)-4:])
		http.Error(w, `{"error": "proxy error"}`, http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}
