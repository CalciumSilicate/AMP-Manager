# Responses WebSocket Stability

This repo includes a real-upstream stability harness for the downstream `GET /v1/responses` WebSocket path.

Command:

```bash
go run ./cmd/responses-ws-stability
```

Required environment variables:

```bash
export AMP_MANAGER_BASE_URL="http://127.0.0.1:16823"
export AMP_ADMIN_USERNAME="admin"
export AMP_ADMIN_PASSWORD="admin123"
export RESPONSES_BASE_URL="https://api.openai.com"
export RESPONSES_API_KEY="sk-..."
export RESPONSES_MODEL="gpt-5"
```

Optional environment variables:

```bash
export RESPONSES_HEADERS_JSON='{"OpenAI-Organization":"org_..."}'
export STABILITY_PREFIX="responses-ws-stability"
export STABILITY_CUSTOM_DOWNSTREAM_KEY="ResponsesWSStabilityKey123456"
export STABILITY_TOPUP_USD="5"
export STABILITY_SINGLE_TURN_SAMPLES="20"
export STABILITY_CONTINUATION_SESSIONS="10"
export STABILITY_LONG_IDLE_SESSIONS="5"
export STABILITY_LONG_IDLE_SECONDS="30"
export STABILITY_CONCURRENCY_LEVELS="1,5,20"
```

Notes:

- `RESPONSES_BASE_URL` is the channel base URL, not the full `/v1/responses` endpoint.
- This defaults to official OpenAI, but any custom Responses-compatible base URL can be tested by changing `RESPONSES_BASE_URL`.
- The harness creates or updates two admin channels:
  - `<prefix>-http`
  - `<prefix>-ws`
- It also updates the current admin user's Amp settings with two model mappings:
  - `<prefix>-http` -> real model on the HTTP channel
  - `<prefix>-ws` -> real model on the WS channel
- Results are written under `.tmp/ws-stability/<timestamp>/`:
  - `summary.json`
  - `samples.jsonl`
  - `report.md`

What the harness checks:

- downstream `/v1/responses` WebSocket success rate
- continuation stability with `previous_response_id`
- long-idle stability on a reused downstream WebSocket session
- moderate concurrency behavior
- request-log transport fields:
  - `downstreamTransport`
  - `upstreamTransport`
  - `transportFallbackReason`
  - `ttfbMs`
  - `latencyMs`

The command exits non-zero if:

- setup fails
- a scenario has client-visible failures
- a scenario records unexpected upstream transport for its expected mode

Typical official OpenAI run:

- `RESPONSES_BASE_URL=https://api.openai.com`
- `RESPONSES_HEADERS_JSON` unset or empty
- expect `U WS` for the WS scenarios and `U HTTP` for the baseline scenario

Typical custom-base run:

- point `RESPONSES_BASE_URL` at the provider's base path
- add any required headers in `RESPONSES_HEADERS_JSON`
- if the provider does not support Responses WebSocket upgrade, the WS scenarios should fail or fall back, and that is the signal to investigate provider compatibility
