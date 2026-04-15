# ============================================
# Stage 1: Build frontend
# ============================================
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# ============================================
# Stage 2: Build backend (cross-compile via xx)
# ============================================
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend
# xx provides cross-compilation helpers for multi-platform builds
COPY --from=tonistiigi/xx / /
ARG TARGETPLATFORM
ARG GO_BUILD_TAGS=""

WORKDIR /app

# Install build deps + cross-compilation toolchain for target
RUN apk add --no-cache clang lld
RUN xx-apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=frontend /app/web/dist ./internal/web/dist

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 sh -c 'xx-go build ${GO_BUILD_TAGS:+-tags "$GO_BUILD_TAGS"} -ldflags="-s -w" -o /out/ampmanager ./cmd/server' && \
    xx-verify /out/ampmanager

# ============================================
# Stage 3: Runtime
# ============================================
FROM alpine:latest
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

COPY --from=backend /out/ampmanager .

EXPOSE 16823
VOLUME ["/app/data"]
CMD ["./ampmanager"]
