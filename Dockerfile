# syntax=docker/dockerfile:1

# ---- web dashboard ----
FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# ---- Go core ----
# TARGETOS/TARGETARCH are set automatically by buildx for each platform in
# --platform linux/amd64,linux/arm64 — confirmed this codebase cross-compiles
# cleanly for both with CGO_ENABLED=0 (see TODO.md Phase 3/8).
FROM golang:1.24-alpine AS core-builder
WORKDIR /src
COPY core/go.mod core/go.sum ./
RUN go mod download
COPY core/ .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/nekodl ./cmd/nekodl

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    addgroup -S nekodl && adduser -S nekodl -G nekodl
WORKDIR /app
COPY --from=core-builder /out/nekodl ./nekodl
COPY --from=web-builder /web/dist ./web/dist
COPY nekodl.docker.json ./nekodl.json
RUN mkdir -p /data && chown -R nekodl:nekodl /app /data
USER nekodl
VOLUME ["/data"]
EXPOSE 6900
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O- http://127.0.0.1:6900/health || exit 1
ENTRYPOINT ["./nekodl"]
CMD ["-config", "/app/nekodl.json"]
