# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
ARG NPM_REGISTRY=https://registry.npmjs.org
RUN --mount=type=cache,target=/root/.npm \
    npm config set registry "${NPM_REGISTRY}" \
    && npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG VCS_REF=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -mod=readonly -buildvcs=false -trimpath \
    -tags="netgo,osusergo" \
    -ldflags="-s -w -buildid= -X main.version=${VERSION} -X main.commit=${VCS_REF}" \
    -o /keysprint .

FROM alpine:3.22 AS certificates
ARG ALPINE_MIRROR=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates \
    && mkdir /data

FROM scratch
WORKDIR /app
COPY --from=certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=certificates --chown=65532:65532 /data /data
COPY --from=backend /keysprint /app/keysprint
COPY --from=frontend /src/frontend/dist /app/public
USER 65532:65532
ENV ADDR=:8080 \
    DB_PATH=/data/keysprint.db \
    PUBLIC_DIR=/app/public \
    TZ=Asia/Shanghai
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/keysprint", "--healthcheck"]
ENTRYPOINT ["/app/keysprint"]
