# AMP Manager

<div align="center">

**AI API 反向代理管理平台**

一站式自托管 AI API 代理服务，支持 [Amp CLI](https://ampcode.com)、[Cherry Studio](https://cherry-ai.com)、Cursor 等客户端。
多渠道智能路由 · 用户分组与订阅计费 · 全面的使用量监控。

[![Docker Image](https://img.shields.io/badge/ghcr.io-amp--manager-blue?logo=docker)](https://ghcr.io/meglinge/amp-manager)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev)

</div>

---

## 功能特性

### 🔄 代理核心

- **多 Provider 支持** — OpenAI、Anthropic Claude、Google Gemini，兼容 `/v1`、`/v1beta` 标准接口
- **智能渠道路由** — 多渠道负载均衡，支持权重、优先级和分组路由策略；同优先级渠道间 Round-Robin 轮询
- **模型白名单** — 渠道可启用白名单模式，支持 `*` 通配符匹配规则，仅暴露指定模型
- **模型映射** — 精确匹配和正则表达式模型名称映射，支持思维级别注入（low/medium/high/xhigh）
- **流式/非流式代理** — 完整支持 SSE 流式响应、Keep-Alive 心跳（15s 间隔）和伪非流模式
- **自动重试** — 可配置重试策略：指数退避 + 抖动，支持 429/5xx 自动重试，首字节超时检测
- **请求过滤** — 可扩展的过滤器框架：Claude Code 身份模拟、缓存 TTL 覆写、系统提示注入
- **协议适配** — 自动检测请求格式（OpenAI Chat/Responses/Claude/Gemini），拒绝跨格式调用
- **响应处理** — 自动解压 gzip/brotli/zstd/deflate，模型名称回写，thinking block 过滤

### 🌐 内置工具

- **本地网页搜索** — 拦截 `webSearch2` 请求，三种模式：上游代理 / 内置免费 / 本地 DuckDuckGo
- **网页内容提取** — 本地处理 `extractWebPageContent`，直接获取网页 HTML 内容
- **广告拦截** — 自动拦截 Amp CLI 内置广告请求，可选将用户余额显示为广告内容

### 👥 管理平台

- **用户系统** — JWT 认证（HS256, 24h 有效期），管理员/普通用户角色，实时权限校验
- **分组管理** — 用户和渠道分组，费率倍率控制，精细化权限：分组用户仅可访问其组内渠道
- **订阅计费** — 双计费源（订阅 + 余额），支持日/周/月/滚动5小时/总量多维度配额限制
- **余额管理** — 微美元精度（1 USD = 1,000,000 micros）整数运算，避免浮点误差
- **API Key 管理** — SHA-256 哈希存储，支持多种认证方式（Bearer/X-Api-Key/x-goog-api-key/query param）
- **仪表盘** — 实时费用统计、热门模型排行、每日趋势图、多 Provider 缓存命中率分析
- **使用量监控** — 请求日志（WebSocket 实时推送）、Token 用量、成本分析，多维度聚合
- **价格管理** — 自动同步 LiteLLM 价格库（6 小时周期 + ETag 缓存），支持手动定价
- **系统设置** — 重试策略、超时配置、缓存 TTL、请求详情开关/归档策略、数据库备份/恢复
- **数据加密** — AES-256-GCM 加密存储上游 API Key，自动检测加密状态

## 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        客户端                                    │
│   Amp CLI  ·  Cherry Studio  ·  Cursor  ·  OpenAI SDK  ·  curl │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTP/SSE
┌────────────────────────────▼────────────────────────────────────┐
│                      AMP Manager                                 │
│                                                                  │
│  ┌──────────┐  ┌───────────┐  ┌──────────┐  ┌───────────────┐  │
│  │ API Key  │→ │ Rate      │→ │ Model    │→ │ Channel       │  │
│  │ Auth     │  │ Limiter   │  │ Mapping  │  │ Router        │  │
│  └──────────┘  └───────────┘  └──────────┘  └───────┬───────┘  │
│                                                       │          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────▼───────┐  │
│  │ Request      │  │ Request      │  │ Proxy Handler         │  │
│  │ Filter Chain │  │ Capture      │  │ (Stream / Non-Stream) │  │
│  └──────────────┘  └──────────────┘  └───────────┬───────────┘  │
│                                                   │              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────▼─────┐       │
│  │ Billing  │  │ Logging  │  │ Usage    │  │ Retry    │       │
│  │ Engine   │  │ (Async)  │  │ Parser   │  │ Transport│       │
│  └──────────┘  └──────────┘  └──────────┘  └────┬─────┘       │
└──────────────────────────────────────────────────┼──────────────┘
                                                   │
┌──────────────────────────────────────────────────▼──────────────┐
│                      上游 Provider                               │
│   OpenAI  ·  Anthropic Claude  ·  Google Gemini  ·  自定义兼容   │
└─────────────────────────────────────────────────────────────────┘
```

### 请求处理流程

1. **认证** — API Key 哈希查找，校验有效期和撤销状态，加载用户配置和分组
2. **限流** — 按 API Key 令牌桶限流（默认 100 rps）
3. **模型映射** — 正则/精确匹配模型名称，注入思维级别参数
4. **渠道路由** — 根据模型名匹配可用渠道，按优先级分层 + 同层 Round-Robin 选择
5. **请求过滤** — Claude Code 身份模拟、缓存 TTL 覆写等过滤器链
6. **请求捕获** — 存储请求头和 Body（512KB 限制）用于调试
7. **代理转发** — `httputil.ReverseProxy` 转发请求，自动适配目标 Provider 认证格式
8. **响应处理** — Token 用量提取（4 种 Provider 格式）、成本计算、模型名回写
9. **计费结算** — 事务性双源扣费（订阅优先 → 余额兜底），写入计费事件
10. **异步日志** — 批量写入数据库（100 条/批，200ms 刷新），WebSocket 实时推送

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go 1.24 · Gin · SQLite (modernc) / PostgreSQL (pgx stdlib) · JWT (golang-jwt/v5) |
| 前端 | React 19 · Vite 6 · TypeScript · Tailwind CSS · shadcn/ui · Recharts · Motion |
| 部署 | Docker 多架构 (amd64/arm64) · Docker Compose |
| CI/CD | GitHub Actions — Docker 镜像发布 + 5 平台二进制发布 |

<details>
<summary>主要 Go 依赖</summary>

| 包 | 用途 |
|---|---|
| `gin-gonic/gin` | HTTP 框架 |
| `modernc.org/sqlite` | 纯 Go SQLite 驱动 |
| `github.com/jackc/pgx/v5/stdlib` | PostgreSQL `database/sql` 驱动 |
| `golang-jwt/jwt/v5` | JWT 认证 |
| `golang.org/x/crypto` | bcrypt 密码哈希 |
| `shopspring/decimal` | 精确十进制运算 (计费) |
| `tidwall/gjson` + `sjson` | JSON 操作 |
| `nhooyr.io/websocket` | WebSocket (实时日志推送) |
| `golang.org/x/time/rate` | 令牌桶限流 |
| `sirupsen/logrus` | 结构化日志 |
| `andybalholm/brotli` + `klauspost/compress` | 响应解压 |

</details>

## 快速开始

### Docker Compose 部署（推荐）

```bash
# 克隆仓库
git clone https://github.com/meglinge/AMP-Manager.git
cd AMP-Manager

# 复制并修改环境变量
cp .env.example .env
# 编辑 .env，必须修改 JWT_SECRET 和 ADMIN_PASSWORD

# 启动服务
docker-compose up -d
```

访问 `http://localhost:16823` 即可使用。

如果你要在 Docker Compose 下直接使用 PostgreSQL：

```bash
DB_TYPE=postgres
DATABASE_URL=postgres://postgres:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable
REDIS_URL=redis://redis:6379/0
docker-compose up -d
```

说明：

1. `postgres` 是 compose 内部服务名。
2. `redis` 是 compose 内部 Redis 服务名。
3. 容器内连接 PostgreSQL 时应使用 `postgres:5432`，不要写 `localhost:5432`。
4. 容器内连接 Redis 时应使用 `redis:6379`，不要写 `localhost:6379`。
5. 当前 `docker-compose.yml` 已为 `ampmanager` 配置 `depends_on + healthcheck`，会等待 PostgreSQL 和 Redis 就绪后再启动应用。

### 二进制部署

从 [GitHub Releases](https://github.com/meglinge/AMP-Manager/releases) 下载对应平台的预编译二进制：

| 平台 | 架构 |
|------|------|
| Linux | amd64, arm64 |
| macOS | amd64 (Intel), arm64 (Apple Silicon) |
| Windows | amd64 |

```bash
# 设置环境变量（生产环境必须）
export JWT_SECRET="your-secret-key-at-least-32-chars"
export ADMIN_PASSWORD="your-secure-password"

# 启动
./ampmanager
```

### 一键管理脚本

```bash
chmod +x manage.sh
./manage.sh
```

提供交互式菜单：启动/停止服务、更新并重启（拉取代码 + 重新构建）、查看日志和状态。

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DB_TYPE` | 数据库类型，支持 `sqlite` / `postgres` | `sqlite` |
| `SQLITE_PATH` | SQLite 数据库文件路径 | `./data/data.db` |
| `DATABASE_URL` | PostgreSQL 连接串（`DB_TYPE=postgres` 时必填） | 空 |
| `REDIS_URL` | Redis 连接串，多实例共享计费热路径时必填 | 空 |
| `REDIS_PREFIX` | Redis Key 前缀 | `ampmanager` |
| `ADMIN_USERNAME` | 管理员用户名 | `admin` |
| `ADMIN_PASSWORD` | 管理员密码（**生产必须修改**） | `admin123` |
| `SERVER_PORT` | 服务端口 | `16823` |
| `JWT_SECRET` | JWT 签名密钥（**生产必须修改**，≥32 字符） | - |
| `JWT_ISSUER` | JWT 签发者 | `ampmanager` |
| `JWT_AUDIENCE` | JWT 受众 | `ampmanager-users` |
| `DATA_ENCRYPTION_KEY` | AES-256 加密密钥（正好 32 字符） | 空（明文存储） |
| `CORS_ALLOWED_ORIGINS` | CORS 允许来源（逗号分隔） | `*`（禁用 CORS） |
| `RATE_LIMIT_AUTH_RPS` | 认证端点每秒请求限制 | `5` |
| `RATE_LIMIT_PROXY_RPS` | 代理端点每秒请求限制 | `100` |
| `ALLOW_INSECURE_DEFAULTS` | 跳过安全校验（仅开发用） | `false` |
| `BILLING_RESERVATION_TTL_SEC` | 计费预留 TTL（秒） | `600` |
| `BILLING_RECONCILE_INTERVAL_SEC` | Redis 与 PostgreSQL 对账间隔（秒） | `60` |
| `BILLING_STREAM_BATCH_SIZE` | Redis Stream projector 批大小 | `100` |
| `BILLING_RECONCILE_BATCH_SIZE` | Reconcile 单轮分页批大小 | 跟随 `BILLING_STREAM_BATCH_SIZE` |
| `BILLING_EXPIRY_BATCH_SIZE` | Expiry 单轮分页批大小 | 跟随 `BILLING_STREAM_BATCH_SIZE` |
| `BILLING_PROJECTOR_WORKERS` | Projector worker 数 | `1` |
| `BILLING_PROJECTOR_CLAIM_IDLE_SEC` | Projector reclaim idle 阈值（秒） | `30` |

> ⚠️ 生产环境**必须**修改 `ADMIN_PASSWORD` 和 `JWT_SECRET`，否则服务将拒绝启动。

## Shared-Billing Benchmark

仓库内置了 `cmd/loadtest-responses`，可直接做基于 PostgreSQL + Redis 的 shared-billing benchmark。

- 操作说明见 [docs/shared-billing-benchmark.md](docs/shared-billing-benchmark.md)
- Linux/macOS 可直接运行 `scripts/run-shared-billing-benchmark.sh`
- Linux/macOS 也可运行 `scripts/run-shared-billing-matrix-suite.sh` 一次完成 workers/batch/claim 三套矩阵并生成摘要
- 现有 JSON 报告可用 `scripts/summarize-shared-billing-report.mjs` 汇总为 markdown
- benchmark 支持 matrix 模式，可直接比较 `projectorWorkers`、batch size、claim idle 等 knobs

## 数据库迁移

项目现在支持在 SQLite 与 PostgreSQL 之间切换运行，并提供双向数据迁移工具。

```bash
# SQLite -> PostgreSQL
go run ./cmd/dbtool migrate --source-type sqlite --source ./data/data.db --target-type postgres --target postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable

# PostgreSQL -> SQLite
go run ./cmd/dbtool migrate --source-type postgres --source postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable --target-type sqlite --target ./data/data.db
```

- 默认会清空目标数据库中的业务表后再导入，可用 `--clear-target=false` 关闭。
- 默认会同时迁移请求详情归档数据，可用 `--with-archive=false` 跳过。
- SQLite 模式仍保留文件级备份/恢复；PostgreSQL 模式请使用 `dbtool migrate` 做导出导入。
- 管理后台已内置同样的迁移能力；CLI 现在只是可选入口。

## 客户端配置

### Amp CLI

在管理平台创建 API Key 后，配置环境变量：

```bash
# Linux / macOS
export AMP_URL="http://your-server:16823"
export AMP_API_KEY="your-api-key"
```

```powershell
# Windows PowerShell（永久生效）
[Environment]::SetEnvironmentVariable("AMP_URL", "http://your-server:16823", "User")
[Environment]::SetEnvironmentVariable("AMP_API_KEY", "your-api-key", "User")
```

### OpenAI 兼容客户端（Cherry Studio、Cursor 等）

| 设置项 | 值 |
|--------|---|
| API Base URL | `http://your-server:16823/v1` |
| API Key | 在管理平台创建的 API Key |

### Gemini SDK 客户端

| 设置项 | 值 |
|--------|---|
| API Base URL | `http://your-server:16823/v1beta` |
| API Key | 通过 `x-goog-api-key` 头或 `?key=` 参数传递 |

## API 概览

### 代理接口

| 路径 | 说明 | 认证 |
|------|------|------|
| `POST /v1/chat/completions` | OpenAI Chat 兼容接口 | API Key |
| `POST /v1/messages` | Anthropic Claude 兼容接口 | API Key |
| `POST /v1/responses` | OpenAI Responses API | API Key |
| `POST /v1beta/models/*:action` | Gemini 兼容接口 | API Key |
| `GET /v1/models` | 模型列表（OpenAI/Claude 格式，自动检测） | 无 |
| `GET /v1beta/models` | 模型列表（Gemini 格式） | 无 |
| `/api/provider/:provider/*` | 多 Provider 代理（Amp CLI 使用） | API Key |
| `GET /threads/:threadID` | 线程跳转到 ampcode.com | 无 |

### 认证接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/manage/auth/register` | 用户注册 |
| POST | `/api/manage/auth/login` | 用户登录 |

### 用户接口（`/api/me/*`）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/me/balance` | 获取余额 |
| GET | `/api/me/dashboard` | 个人仪表盘 |
| GET/PUT | `/api/me/amp/settings` | 代理设置（上游地址、模型映射、搜索模式等） |
| POST | `/api/me/amp/settings/test` | 测试上游连接 |
| CRUD | `/api/me/amp/api-keys` | API Key 管理 |
| GET | `/api/me/amp/request-logs` | 请求日志（分页、筛选） |
| GET | `/api/me/amp/usage/summary` | 用量统计（按日/模型/Key 聚合） |
| GET | `/api/me/billing/state` | 计费状态（余额 + 订阅 + 配额余量） |
| PUT | `/api/me/billing/priority` | 设置计费优先源 |
| PUT | `/api/me/password` | 修改密码 |
| PUT | `/api/me/username` | 修改用户名 |

### 管理员接口（`/api/admin/*`）

| 方法 | 路径 | 说明 |
|------|------|------|
| CRUD | `/api/admin/channels` | 渠道管理（类型、端点、密钥、权重、优先级、分组、白名单） |
| POST | `/api/admin/channels/:id/test` | 测试渠道连接 |
| POST | `/api/admin/channels/:id/fetch-models` | 从上游获取可用模型 |
| CRUD | `/api/admin/groups` | 分组管理（费率倍率） |
| GET | `/api/admin/users` | 用户列表 |
| POST | `/api/admin/users/:id/topup` | 用户充值 |
| PATCH | `/api/admin/users/:id/group` | 设置用户分组 |
| CRUD | `/api/admin/subscriptions/plans` | 订阅计划管理（限额、窗口模式） |
| POST | `/api/admin/subscriptions/assign` | 分配订阅给用户 |
| CRUD | `/api/admin/model-metadata` | 模型元数据（上下文长度、最大 Token） |
| GET | `/api/admin/prices` | 价格列表 |
| POST | `/api/admin/prices/refresh` | 从 LiteLLM 同步价格 |
| GET | `/api/admin/dashboard` | 全局仪表盘（所有用户汇总） |
| GET | `/api/admin/request-logs` | 全局请求日志 |
| WS | `/api/admin/request-logs/ws` | WebSocket 实时日志推送 |
| * | `/api/admin/system/*` | 系统设置（数据库、重试、超时、缓存、监控开关） |

## 数据模型

<details>
<summary>展开查看所有数据表</summary>

| 表 | 说明 | 关键字段 |
|---|------|---------|
| `users` | 用户账户 | username, password_hash, is_admin, balance_micros |
| `groups` | 分组 | name, rate_multiplier |
| `user_groups` | 用户↔分组（M:N） | user_id, group_id |
| `channels` | 上游渠道 | type, base_url, api_key, weight, priority, model_whitelist |
| `channel_groups` | 渠道↔分组（M:N） | channel_id, group_id |
| `channel_models` | 渠道可用模型 | channel_id, model_id, display_name |
| `user_amp_settings` | 用户代理配置 | upstream_url, model_mappings_json, web_search_mode, native_mode |
| `user_api_keys` | API 密钥 | key_hash, api_key (加密), prefix, expires_at, revoked_at |
| `request_logs` | 请求日志 | model, tokens, cost_micros, latency_ms, billing_status |
| `request_log_details` | 请求详情热数据 | request_headers, request_body, response_headers, response_body |
| `request_log_details_archive` | 请求详情归档（SQLite 为独立归档库，PostgreSQL 为同库归档表） | request_headers, request_body, response_headers, response_body |
| `subscription_plans` | 订阅计划 | name, enabled |
| `subscription_plan_limits` | 计划限额 | limit_type, window_mode, limit_micros |
| `user_subscriptions` | 用户订阅 | plan_id, starts_at, expires_at, status |
| `user_billing_settings` | 计费优先设置 | primary_source, secondary_source |
| `billing_events` | 计费事件 | source, event_type, amount_micros |
| `model_metadata` | 模型元数据 | model_pattern, context_length, max_completion_tokens |
| `model_prices` | 模型价格 | model, price_data (input/output/cache per token) |
| `system_config` | 系统配置（KV） | key, value |

</details>

## 本地开发

### 一键启动（推荐）

```powershell
# Windows
.\dev.ps1
```

```bash
# Linux / macOS
./dev.sh
```

脚本会自动完成三件事：

1. 使用 `docker-compose.dev.yml` 启动本地 PostgreSQL。
2. 启动 Vite 前端热更新（`http://localhost:5274`）。
3. 启动 Air 后端热更新（`http://localhost:16823`）。

开发模式下，数据库选择会持久化到 `data/config.json`：

1. 文件不存在时，默认使用本地 PostgreSQL。
2. 你在管理后台切换到 SQLite 或 PostgreSQL 后，下一次 `dev.ps1` / `dev.sh` 启动会自动沿用。

### 后端

```bash
# 设置环境变量跳过安全检查
export ALLOW_INSECURE_DEFAULTS=true  # Linux/macOS
$env:ALLOW_INSECURE_DEFAULTS="true"  # Windows PowerShell

# 切换到 PostgreSQL 运行
export DB_TYPE=postgres
export DATABASE_URL="postgres://postgres:mysecretpassword@localhost:5432/ampmanager?sslmode=disable"

go run ./cmd/server
# 服务启动在 http://localhost:16823
```

支持 [Air](https://github.com/cosmtrek/air) 热重载：

```bash
air
```

### 前端

```bash
cd web
pnpm install
pnpm dev
# 开发服务器启动在 http://localhost:5274
# API 请求自动代理到 http://localhost:16823
```

### 单独启动 PostgreSQL

```bash
docker compose -f docker-compose.dev.yml up -d postgres
```

### 构建

使用项目提供的构建脚本（自动完成前端构建 → 复制到嵌入目录 → 使用 `embed_frontend` 构建后端）：

```bash
# Windows
.\build.ps1

# Linux/macOS
./build.sh
```

<details>
<summary>手动构建步骤</summary>

```bash
# 1. 构建前端
cd web && pnpm install && pnpm run build && cd ..

# 2. 复制到嵌入目录（仅发布 / 嵌入构建需要）
# Windows:
xcopy /E /I /Y "web\dist" "internal\web\dist"
# Linux/macOS:
cp -r web/dist internal/web/dist

# 3. 构建后端
# 开发 / 测试：默认从磁盘 web/dist 读取；如果缺失会返回一个明确提示页
go build -ldflags="-s -w" -o ampmanager ./cmd/server

# 发布 / 单文件二进制：使用显式嵌入模式
go build -tags embed_frontend -ldflags="-s -w" -o ampmanager ./cmd/server
```

</details>

## 目录结构

```
AMPManager/
├── cmd/server/              # 程序入口 (main.go)
├── internal/
│   ├── amp/                 # 代理核心 (43 个文件)
│   │   ├── routes.go        #   路由注册与中间件链
│   │   ├── channel_router.go#   渠道路由：模型提取、格式检测、渠道选择
│   │   ├── proxy.go         #   代理处理器：上游代理 + 渠道代理
│   │   ├── middleware.go    #   认证、广告拦截、搜索策略等中间件
│   │   ├── model_mapping.go #   模型映射：正则匹配、思维级别注入
│   │   ├── retry_transport.go#  重试传输层：指数退避、首字节超时
│   │   ├── web_search.go    #   网页搜索：DuckDuckGo 本地搜索
│   │   ├── stream_handler.go#   SSE 流式处理与 Keep-Alive
│   │   ├── token_extractor.go#  Token 用量提取 (4 种 Provider)
│   │   └── ...              #   更多：响应重写、伪非流、错误分类等
│   ├── billing/             # 计费模块：价格存储、成本计算器、LiteLLM 同步
│   ├── config/              # 配置管理：环境变量加载与安全校验
│   ├── crypto/              # 加密工具：AES-256-GCM
│   ├── database/            # SQLite/PostgreSQL：建表、版本化迁移、方言适配、连接封装
│   ├── handler/             # HTTP 处理器：管理员/用户/认证 API
│   ├── middleware/          # 通用中间件：JWT 认证、IP 限流、CORS
│   ├── model/               # 数据模型：16+ 表定义
│   ├── repository/          # 数据访问层：SQL 查询、事务管理
│   ├── response/            # 统一响应格式
│   ├── router/              # 路由注册
│   ├── service/             # 业务逻辑：用户、渠道、分组、计费、订阅
│   ├── translator/          # 请求过滤器框架：Claude Code 模拟、缓存 TTL
│   ├── util/                # 工具函数：JSON 思维预算、模型能力检测
│   └── web/                 # 前端静态文件加载（默认读 web/dist，发布时可 embed）
├── web/                     # 前端源码
│   └── src/
│       ├── api/             #   API 客户端 (auth, admin, me, channels, etc.)
│       ├── components/      #   UI 组件 (25 个 shadcn/ui 基础组件 + 自定义)
│       ├── lib/             #   工具库
│       └── pages/           #   18 个页面
│           ├── Overview.tsx  #     用户仪表盘 (余额、图表、缓存命中率)
│           ├── Channels.tsx  #     渠道管理 (CRUD、测试、模型获取)
│           ├── UserManagement.tsx # 用户管理 (角色、充值、订阅)
│           ├── RequestLogs.tsx#    请求日志 (WebSocket 实时)
│           └── ...           #     更多：分组、订阅计划、系统设置等
├── CLIProxyAPI/             # 参考实现 (只读)
├── data/                    # SQLite 数据库与归档文件 (运行时自动创建)
├── .github/workflows/       # CI/CD (Docker 镜像 + 二进制发布)
├── docker-compose.yml       # 生产 Docker Compose
├── docker-compose.dev.yml   # 开发 PostgreSQL Docker Compose
├── Dockerfile               # 3 阶段多架构构建
├── build.ps1 / build.sh     # 一键构建脚本
├── dev.ps1 / dev.sh         # 一键开发脚本
└── manage.sh                # Docker 管理脚本
```

## 前端页面

| 页面 | 说明 | 角色 |
|------|------|------|
| 登录/注册 | Glassmorphism 风格，动态渐变背景 | 公开 |
| 概览仪表盘 | 余额、今日/本周/本月统计、30 天趋势图、Top 模型饼图、缓存命中率 | 用户 |
| 代理设置 | 上游地址/密钥、原生模式、模型映射编辑器、搜索模式、广告余额显示 | 用户 |
| API 密钥 | 创建/查看/删除密钥，一键复制 CLI 配置命令 | 用户 |
| 请求日志 | 分页表格 + WebSocket 实时更新，筛选器（用户/密钥/模型/日期范围） | 用户/管理员 |
| 用量统计 | 按日/模型/用户聚合，自动刷新（5/10/30/60s） | 用户/管理员 |
| 模型浏览 | 按类型分组展示所有可用模型 | 用户 |
| 账户设置 | 余额查看、计费优先设置、订阅详情（进度条）、修改密码/用户名 | 用户 |
| 全局仪表盘 | 总余额、用户数、全局统计/趋势/缓存率 | 管理员 |
| 渠道管理 | CRUD 渠道（类型/端点/密钥/权重/优先级/分组/白名单/自定义头），测试连接，获取模型 | 管理员 |
| 用户管理 | 列表/删除/重置密码/设管理员/充值/分配订阅/设分组 | 管理员 |
| 分组管理 | CRUD 分组，设置费率倍率 | 管理员 |
| 订阅计划 | CRUD 计划，多维度限额（日/周/月/滚动5h/总量 × 固定/滑动窗口） | 管理员 |
| 模型元数据 | CRUD 模型元数据（模式匹配、上下文长度、最大 Token） | 管理员 |
| 价格管理 | LiteLLM 价格表，搜索/筛选/手动刷新 | 管理员 |
| 系统设置 | 数据库备份/恢复、重试策略、请求监控开关/归档策略、缓存 TTL、超时配置 | 管理员 |

## 默认账户

首次启动自动创建管理员：
- 用户名：`admin`
- 密码：`admin123`（可通过 `ADMIN_PASSWORD` 环境变量修改）

> ⚠️ 生产环境**必须**修改默认密码和 `JWT_SECRET`，否则服务将拒绝启动。设置 `ALLOW_INSECURE_DEFAULTS=true` 可跳过此检查（仅限开发环境）。

## CI/CD

| 工作流 | 触发条件 | 产物 |
|--------|---------|------|
| `docker-release.yml` | push 到 `main` 或 `v*` 标签 | `ghcr.io/meglinge/amp-manager` (linux/amd64 + arm64) |
| `docker-release-dev.yml` | push 到 `dev` | `ghcr.io/meglinge/amp-manager-dev` (linux/amd64 + arm64) |
| `binary-release.yml` | `v*` 标签 | 5 平台预编译二进制 (GitHub Release) |

## 致谢

本项目的代理核心实现参考了 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)，感谢原作者的开源贡献。AMPManager 在其基础上扩展了多用户管理、渠道路由、订阅计费、React 管理界面等功能。

## 许可证

[MIT License](LICENSE) © 2026 乔浩
