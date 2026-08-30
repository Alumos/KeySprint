# syntax=docker/dockerfile:1
# Build arguments keep slow VPS builds on a nearby package mirror. Override them
# at build time for deployments that have fast access to the official registries.
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
ARG NPM_REGISTRY=https://registry.npmmirror.com
RUN npm config set registry "${NPM_REGISTRY}" \
    && npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY backend/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /keysprint .

FROM alpine:3.21
ARG ALPINE_MIRROR=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata \
    && addgroup -S keysprint \
    && adduser -S -G keysprint keysprint
WORKDIR /app
COPY --from=backend /keysprint /app/keysprint
COPY --from=frontend /src/frontend/dist /app/public
RUN mkdir /data && chown keysprint:keysprint /data
USER keysprint
ENV ADDR=:8080 DB_PATH=/data/keysprint.db PUBLIC_DIR=/app/public TZ=Asia/Shanghai
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/app/keysprint"]
