# syntax=docker/dockerfile:1.7
# ---------- 前端构建 ----------
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- Go 构建（静态二进制，无 CGO） ----------
FROM golang:1.23-alpine AS build
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# 用真实前端产物替换占位页面，embed 进二进制
COPY --from=web /src/web/dist ./internal/static/dist/
RUN go build -trimpath -ldflags "-s -w \
    -X github.com/laoXuHome/LocalHub/internal/buildinfo.Version=${VERSION} \
    -X github.com/laoXuHome/LocalHub/internal/buildinfo.Commit=${COMMIT} \
    -X github.com/laoXuHome/LocalHub/internal/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/localhub ./cmd/localhub

# ---------- 最终镜像：distroless（无 Shell / 无包管理器） ----------
FROM gcr.io/distroless/static-debian12:latest

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="LocalHub" \
      org.opencontainers.image.description="本地优先的 NAS 书签与服务面板（P0 基础设施）" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.source="https://github.com/laoXuHome/LocalHub"

COPY --from=build /out/localhub /localhub

# 容器内数据根固定为 /data；宿主机路径由 compose 的 volume 决定
ENV LOCALHUB_DATA_DIR=/data \
    LOCALHUB_HTTP_ADDR=0.0.0.0 \
    LOCALHUB_HTTP_PORT=11030 \
    TZ=Asia/Shanghai \
    PUID=1000 \
    PGID=1000

EXPOSE 11030

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/localhub", "--healthcheck"]

ENTRYPOINT ["/localhub"]
