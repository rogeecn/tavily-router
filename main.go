package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	var (
		configPath string
	)
	flag.StringVar(&configPath, "config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	proxy, err := NewTavilyProxy(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化代理失败: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Tavily Router 启动")
	log.Printf("  监听地址: %s", cfg.Listen)
	log.Printf("  上游地址: %s", cfg.Upstream)
	log.Printf("  API Keys: %d 个 (round-robin)", proxy.rotator.Len())
	log.Printf("  入站认证: 已启用 (%d 个 token)", len(cfg.Auth))

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: proxy,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
