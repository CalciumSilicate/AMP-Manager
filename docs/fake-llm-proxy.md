# Fake LLM Proxy

独立启动一个用于压测 AMP Manager 长连接并发的虚拟上游：

```bash
go run ./cmd/fake-llm-proxy -listen :8099
```

默认能力：

- 支持模型：`gpt-5.4`、`gpt-5.3-codex`、`gpt-5.2`、`gpt-5.4-mini`
- HTTP `POST /v1/responses`
- SSE：请求体带 `"stream": true`
- WebSocket：`GET /v1/responses`
- 模型枚举：`GET /v1/models`
- 健康检查：`GET /healthz`

默认模拟参数：

- TTFB 主要落在 `1.5s ~ 6s`，尾部延伸到约 `7s`
- 流式输出速率 `30 ~ 60 tok/s`
- 输出总时长按输入长度线性放大，并钳制到 `10s ~ 200s`
- 输出 token 为随机 ASCII 字符串

常用调参：

```bash
go run ./cmd/fake-llm-proxy \
  -listen :8099 \
  -ttfb-min 1500ms \
  -ttfb-main-max 6s \
  -ttfb-tail-max 7s \
  -tps-min 30 \
  -tps-max 60 \
  -output-min 10s \
  -output-max 200s
```

说明：

- `cmd/loadtest-responses` 的内置 mock 也复用了同一套 Responses 构造逻辑。
- 这个服务不校验 API Key，目标是尽量把机器资源用于并发连接与长流测试。
