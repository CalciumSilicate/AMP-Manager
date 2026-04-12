#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
WEB_DIR="$ROOT_DIR/web"
COMPOSE_FILE="$ROOT_DIR/docker-compose.dev.yml"
AIR_CONFIG="$ROOT_DIR/.air.unix.toml"
FRONTEND_PORT=5274
BACKEND_PORT=16823
TMP_DIR="$ROOT_DIR/tmp"
AIR_LOG="$TMP_DIR/air-dev.log"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo -e "${RED}[错误] 未找到 $1，请先安装${NC}"
    exit 1
  fi
}

ensure_pnpm() {
  if ! command -v pnpm >/dev/null 2>&1; then
    echo -e "${YELLOW}[信息] 未找到 pnpm，正在安装...${NC}"
    npm install -g pnpm
  fi
}

ensure_air() {
  if command -v air >/dev/null 2>&1; then
    command -v air
    return
  fi

  echo -e "${YELLOW}[信息] 未找到 air，正在安装...${NC}" >&2
  go install github.com/air-verse/air@latest
  local gobin
  gobin="$(go env GOBIN)"
  if [ -z "$gobin" ]; then
    gobin="$(go env GOPATH)/bin"
  fi
  if [ -x "$gobin/air" ]; then
    echo "$gobin/air"
    return
  fi

  echo -e "${RED}[错误] air 安装完成但未找到可执行文件，请确认 GOBIN 或 GOPATH/bin 已加入 PATH${NC}" >&2
  exit 1
}

wait_for_postgres() {
  for _ in $(seq 1 60); do
    if (echo >/dev/tcp/127.0.0.1/5432) >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  echo -e "${RED}[错误] 等待 PostgreSQL 就绪超时${NC}"
  exit 1
}

wait_for_port() {
  local port="$1"
  local label="$2"
  local timeout="${3:-20}"

  for _ in $(seq 1 "$timeout"); do
    if command -v lsof >/dev/null 2>&1; then
      if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        return
      fi
    elif (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done

  echo -e "${RED}[错误] 等待 ${label} (${port}) 就绪超时${NC}"
  if [ -f "$AIR_LOG" ]; then
    echo -e "${YELLOW}[调试] 最近的后端日志:${NC}"
    tail -n 40 "$AIR_LOG" || true
  fi
  exit 1
}

port_in_use() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1
}

ensure_port_free() {
  local port="$1"
  local label="$2"

  if port_in_use "$port"; then
    echo -e "${RED}[错误] ${label}端口 ${port} 已被占用${NC}"
    if command -v lsof >/dev/null 2>&1; then
      lsof -nP -iTCP:"$port" -sTCP:LISTEN || true
    fi
    exit 1
  fi
}

cleanup() {
  if [ -n "${FRONTEND_PID:-}" ]; then
    kill "$FRONTEND_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "${BACKEND_PID:-}" ]; then
    kill "$BACKEND_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

echo ""
echo "=============================="
echo "   AMP Manager 开发启动脚本"
echo "=============================="
echo ""

require_command node
ensure_pnpm
require_command go
require_command docker
AIR_BIN="$(ensure_air)"

if [ ! -f "$AIR_CONFIG" ]; then
  echo -e "${RED}[错误] 未找到 Unix Air 配置: $AIR_CONFIG${NC}"
  exit 1
fi

mkdir -p "$TMP_DIR"

ensure_port_free "$FRONTEND_PORT" "前端"
ensure_port_free "$BACKEND_PORT" "后端"

echo -e "${GREEN}[1/4] 启动 PostgreSQL 容器...${NC}"
docker compose -f "$COMPOSE_FILE" up -d postgres
wait_for_postgres

echo ""
echo -e "${GREEN}[2/4] 安装前端依赖...${NC}"
cd "$WEB_DIR"
pnpm install
cd "$ROOT_DIR"

POSTGRES_USER_VALUE="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD_VALUE="${POSTGRES_PASSWORD:-mysecretpassword}"
POSTGRES_DB_VALUE="${POSTGRES_DB:-ampmanager}"

export ALLOW_INSECURE_DEFAULTS=true
export AMP_DEV_RUNTIME_DB_CONFIG=true
export DB_TYPE=postgres
export DATABASE_URL="postgres://${POSTGRES_USER_VALUE}:${POSTGRES_PASSWORD_VALUE}@localhost:5432/${POSTGRES_DB_VALUE}?sslmode=disable"
export CORS_ALLOWED_ORIGINS=http://localhost:${FRONTEND_PORT}
export SERVER_PORT=${BACKEND_PORT}

echo ""
echo -e "${GREEN}[3/4] 启动前端热更 (Vite)...${NC}"
cd "$WEB_DIR"
pnpm run dev -- --host 0.0.0.0 --strictPort &
FRONTEND_PID=$!
cd "$ROOT_DIR"
wait_for_port "$FRONTEND_PORT" "前端开发服务器"

echo ""
echo -e "${GREEN}[4/4] 启动后端热更 (Air)...${NC}"
"$AIR_BIN" -c "$AIR_CONFIG" >"$AIR_LOG" 2>&1 &
BACKEND_PID=$!
wait_for_port "$BACKEND_PORT" "后端开发服务器"

echo ""
echo "=============================="
echo "   前端: http://localhost:${FRONTEND_PORT}"
echo "   后端: http://localhost:${BACKEND_PORT}"
echo "   PostgreSQL 容器: localhost:5432"
echo "   默认数据库模式: 读取 ./data/config.json；首次缺省为 PostgreSQL"
echo "   Air 日志: $AIR_LOG"
echo "=============================="
echo ""

wait "$FRONTEND_PID" "$BACKEND_PID"
