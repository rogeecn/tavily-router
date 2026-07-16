# Tavily Router

Tavily API Key 轮询代理。读取 YAML 配置文件中的多个 API Key，以 round-robin 方式轮询转发请求到 Tavily API。

## 快速开始

```bash
# 1. 复制配置文件并填入你的 API Keys
cp config.example.yaml config.yaml
vim config.yaml

# 2. 编译
go build -o tavily-router .

# 3. 启动
./tavily-router -config config.yaml
```

## 配置说明

```yaml
# 监听地址
listen: "0.0.0.0:8787"

# Tavily API 上游地址
upstream: "https://api.tavily.com"

# API Keys (round-robin 轮询)
api_keys:
  - "tvly-xxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  - "tvly-yyyyyyyyyyyyyyyyyyyyyyyyyyyy"
```

## 使用方式

启动后，将原本发往 `https://api.tavily.com` 的请求改为发往 `http://<listen_addr>` 即可。

```bash
# 原始请求
curl -X POST https://api.tavily.com/search \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer tvly-yourkey" \
  -d '{"query": "latest AI news"}'

# 通过 Router (不需要带 key，Router 会自动替换)
curl -X POST http://localhost:8787/search \
  -H "Content-Type: application/json" \
  -d '{"query": "latest AI news"}'
```

## 支持的 Tavily 端点

Router 是透明反向代理，支持所有 Tavily API 端点：

- `POST /search`
- `POST /extract`
- `POST /crawl`
- `POST /map`
- `POST /research`

## 命令行参数

```
./tavily-router -config config.yaml    # 指定配置文件路径 (默认 config.yaml)
```
