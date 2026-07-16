package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen  string   `yaml:"listen"`
	Upstream string  `yaml:"upstream"`
	APIKeys []string `yaml:"api_keys"`
}

type KeyRotator struct {
	keys []string
	idx  uint64
	mu   sync.Mutex
}

func (r *KeyRotator) Next() string {
	if len(r.keys) == 0 {
		return ""
	}
	i := atomic.AddUint64(&r.idx, 1)
	return r.keys[(i-1)%uint64(len(r.keys))]
}

func (r *KeyRotator) Len() int {
	return len(r.keys)
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = "0.0.0.0:8787"
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "https://api.tavily.com"
	}
	if len(cfg.APIKeys) == 0 {
		return nil, fmt.Errorf("配置文件中至少需要一个 api_key")
	}

	return &cfg, nil
}
