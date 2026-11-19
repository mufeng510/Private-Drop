# syntax=docker/dockerfile:1.7
# -----------------------------------------------------------------------------
# Private Drop — 单镜像多阶段构建
#   Stage 1 frontend : Node 24 构建 React SPA -> web/dist
#   Stage 2 backend  : Go 1.27 编译 server / resetpwd 两个静态二进制
#                      （web/dist 在此阶段被 go:embed 嵌入二进制）
#   Stage 3 runtime  : 精简 Alpine，非 root 运行，VOLUME /data
#
# 可复现性说明：
#   - VERSION / COMMIT / BUILD_DATE 由构建方以 --build-arg 传入，写入 OCI 标签。
#   - 发布流水线同时打 semver 与 commit SHA 标签（见 .github/workflows/ci.yml）。
#   - 如需位级可复现，请把 node/golang/alpine 固定到具体 digest（替换为
#     `node:24-alpine@sha256:...` 形式），并在发布说明中记录摘要。
# -----------------------------------------------------------------------------
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# ---------------------------- Stage 1: frontend ------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/web
# 先只拷贝依赖清单，最大化 npm 层缓存命中率
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ----------------------------- Stage 2: backend ------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
# git: 少数模块走 VCS 拉取时需要；ca-certificates: HTTPS 模块代理需要
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# 用 Stage 1 的产物覆盖仓库里可能存在的占位 web/dist
COPY --from=frontend /src/web/dist/ /src/web/dist/
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH
# CGO_ENABLED=0 -> 纯静态二进制，可在 Alpine/distroless 上直接运行
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
      go build -trimpath -ldflags="-s -w" -o /out/private-drop ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-$(go env GOARCH)} \
      go build -trimpath -ldflags="-s -w" -o /out/private-drop-resetpwd ./cmd/resetpwd

# ----------------------------- Stage 3: runtime ------------------------------
FROM alpine:3.22 AS runtime
# tzdata: TZ 生效；wget(busybox): 健康检查；ca-certificates: 未来出站 HTTPS 预留
RUN apk add --no-cache ca-certificates tzdata wget su-exec \
 && addgroup -g 10001 -S pd \
 && adduser -u 10001 -S -G pd -H -D pd \
 && mkdir -p /data \
 && chown -R 10001:10001 /data

WORKDIR /app
COPY --from=backend --chown=10001:10001 /out/private-drop /usr/local/bin/private-drop
COPY --from=backend --chown=10001:10001 /out/private-drop-resetpwd /usr/local/bin/private-drop-resetpwd

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ENV PORT=8080 \
    DATA_DIR=/data \
    TZ=UTC \
    VERSION=${VERSION}
LABEL org.opencontainers.image.title="Private Drop" \
      org.opencontainers.image.description="Self-hosted single-user NAS message drop" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/OWNER/private-drop"

# 数据目录独立挂载，便于 NAS 备份（见 docs/BACKUP_RESTORE.md）
VOLUME ["/data"]
EXPOSE 8080

# 使用公开的未鉴权端点做存活性检查；shell 形式会在运行时展开 ${PORT}
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/api/auth/status" || exit 1

# 权限：入口脚本仅在需要时修复挂载目录属主，随后降到 UID 10001 运行
COPY --chown=10001:10001 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

# Start as root only to repair bind-mount ownership, then drop to UID 10001.
USER root
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
