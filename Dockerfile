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

# ---- yt-dlp (built from source so local patches can apply) ----
# Pinned, not "latest" — an unannounced yt-dlp upgrade mid-deployment could
# change CLI flags or the JSON progress fields ytdlpengine parses. The
# periodic background update check (main.go) only *reports* that a newer
# version exists; bumping this ARG is the deliberate act that actually
# changes the bundled binary.
#
# Built from yt-dlp's own git tag (not installed from PyPI) so any patch
# files dropped in docker/patches/yt-dlp/ apply before NekoDL ships it —
# see that directory's README for the patch workflow. There are no patches
# today, so this is currently equivalent to installing the stock release.
FROM python:3.12-alpine AS ytdlp-builder
ARG YTDLP_VERSION=2026.06.09
RUN apk add --no-cache git
WORKDIR /ytdlp-src
RUN git clone --depth 1 --branch "${YTDLP_VERSION}" https://github.com/yt-dlp/yt-dlp.git .
COPY docker/patches/yt-dlp/ /tmp/patches/
RUN set -eu; \
    for p in /tmp/patches/*.patch; do \
        [ -e "$p" ] || continue; \
        echo "applying patch: $p"; \
        git apply "$p"; \
    done
RUN pip wheel --no-deps --wheel-dir /out .

# ---- runtime ----
FROM alpine:3.20
COPY --from=ytdlp-builder /out/*.whl /tmp/
RUN apk add --no-cache ca-certificates su-exec python3 ffmpeg py3-pip && \
    pip3 install --no-cache-dir --break-system-packages /tmp/*.whl && \
    rm -f /tmp/*.whl && \
    addgroup -S nekodl && adduser -S nekodl -G nekodl
WORKDIR /app
COPY --from=core-builder /out/nekodl ./nekodl
COPY --from=web-builder /web/dist ./web/dist
COPY nekodl.docker.json ./nekodl.json
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh && mkdir -p /data && chown -R nekodl:nekodl /app /data
VOLUME ["/data"]
EXPOSE 6900
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O- http://127.0.0.1:6900/health || exit 1
# Stays root here on purpose — the entrypoint fixes /data ownership (which a
# mounted volume can override regardless of what the image set) and then
# drops to the nekodl user itself via su-exec before running anything else.
ENTRYPOINT ["/entrypoint.sh"]
CMD ["./nekodl", "-config", "/app/nekodl.json"]
