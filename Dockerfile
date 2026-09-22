# ============================================================================
# GOLD-MD — Dockerfile (ARM64 / multi-arch)
#
# ⚠️  ARM64 FIX (2026-09-22): Snapdeploy (AWS Fargate) runs on ARM64 (Graviton).
#     Pehle ye Dockerfile `go build` ko builder ke arch pe chhod deta tha →
#     amd64 binary banti thi → ARM platform pe "exec format error" aata tha.
#     AB: binary explicitly ARM64 ke liye cross-compile hoti hai (GOARCH=arm64)
#     aur runtime base bhi ARM64 hai. Isse image ARM pe natively chalti hai.
#
# Multi-stage source build: Stage-1 golang:1.26-bookworm me source se binary
# compile hoti hai (vendored deps, CGO_ENABLED=0, modernc.org/sqlite — CGO-free).
# Stage-2 debian:bookworm-slim (ARM64) runtime.
#
# Full command parity: ffmpeg (play/sticker/tomp3), python3+Pillow (pdf),
# libreoffice (document->pdf), poppler (pdf text), jpegoptim/pngquant (compress).
# ============================================================================

# ── Stage 1: builder (source -> static ARM64 CGO-free binary) ────────────────
FROM golang:1.26-bookworm AS builder
WORKDIR /build
# Poora source tree copy (src + gold-cmds + vendor + go.mod/go.sum). Vendor dir
# repo me committed hai — -mod=vendor se koi network fetch nahi, whatsmeow
# vendor patches (pair-code.go) automatically apply hote hain.
COPY . .
# build-stamp: 20260922-arm64 source-build (ARM arch fix)
RUN echo "GOLD-MD build-stamp 20260922-arm64 source-build"
# ARM64 cross-compile: GOOS=linux GOARCH=arm64. CGO off → pure Go static binary,
# koi libc dependency nahi (Snapdeploy ARM Fargate pe natively chalti hai).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -ldflags="-s -w" -o /build/gold-md ./src

# ── Stage 2: runtime (ARM64) ─────────────────────────────────────────────────
# --platform=linux/arm64 → ARM64 base image pull hoti hai (Snapdeploy ARM).
FROM --platform=linux/arm64 debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates libc6 ffmpeg python3 python3-pil \
        jpegoptim pngquant poppler-utils \
        libreoffice-writer libreoffice-calc libreoffice-impress \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /build/gold-md ./gold-md
COPY servers.json .
COPY start.sh .
# .env REMOVED — all credentials are hardcoded in source (storj.go / config.go)
RUN chmod +x ./gold-md && chmod +x ./start.sh && mkdir -p nexstore/pairing

# ngrok agent (ARM64) — optional runtime tool (NGROK_AUTHTOKEN + NGROK_DOMAIN
# env vars container runtime pe set karo, image me bake NAHI karna).
# NON-FATAL: download fail ho to ngrok SKIP — build/pass kharab NAHI hoga.
RUN set -eux; \
    if curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 20 \
            https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-arm64.tgz \
            -o /tmp/ngrok.tgz 2>/dev/null; then \
        tar xzf /tmp/ngrok.tgz -C /usr/local/bin; \
        rm -f /tmp/ngrok.tgz; \
        chmod +x /usr/local/bin/ngrok; \
        /usr/local/bin/ngrok --version || true; \
    else \
        echo "WARN: ngrok download FAILED (proxy/CDN block) — skipping ngrok install, build continue"; \
    fi

ENV PORT=11221
# ── SUPERVISOR_ENABLED=1 (owner order — restart/reconnect loop FIX) ──
# start.sh khud supervisor hai (bot exit pe 5s baad single fresh process uthata
# hai). Is env ke bina memoryWatchdog cgroup memory.current (page-cache samet)
# dekhta hai → 512MB container pe bar bar self-restart → 2 processes same
# WhatsApp session pe → "stream replaced" kick-kick loop.
ENV SUPERVISOR_ENABLED=1
EXPOSE 11221
CMD ["./start.sh"]
