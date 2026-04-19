#!/bin/bash

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo ""
echo "=============================="
echo "   AMP Manager 编译脚本"
echo "=============================="
echo ""

# 检查 Node.js
if ! command -v node &> /dev/null; then
    echo -e "${RED}[错误] 未找到 Node.js，请先安装 Node.js${NC}"
    exit 1
fi

# 检查 pnpm
if ! command -v pnpm &> /dev/null; then
    echo -e "${YELLOW}[信息] 未找到 pnpm，正在安装...${NC}"
    npm install -g pnpm
fi

# 检查 Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}[错误] 未找到 Go，请先安装 Go${NC}"
    exit 1
fi

echo -e "${GREEN}[1/5] 安装前端依赖...${NC}"
cd web
pnpm install

echo ""
echo -e "${GREEN}[2/5] 编译前端...${NC}"
pnpm run build
cd ..

echo ""
echo -e "${GREEN}[3/5] 复制前端文件到嵌入目录...${NC}"
mkdir -p internal/web/dist
cp -r web/dist/* internal/web/dist/

echo ""
echo -e "${GREEN}[4/5] 编译后端二进制文件...${NC}"
go build -tags embed_frontend -ldflags="-s -w" -o ampmanager ./cmd/server

echo ""
echo -e "${GREEN}[5/5] 生成 Docker Compose 本地静态二进制...${NC}"
mkdir -p .compose-local
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go build -tags embed_frontend -ldflags="-s -w" -o .compose-local/ampmanager-static ./cmd/server

echo ""
echo "=============================="
echo -e "${GREEN}  编译完成！${NC}"
echo "  输出文件: ampmanager"
echo "  Compose 本地二进制: .compose-local/ampmanager-static"
echo "=============================="
echo ""
