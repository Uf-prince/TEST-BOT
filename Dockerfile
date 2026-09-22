# ============================================================================
# GOLD-MD — ROOT Dockerfile (Render / Back4app / Railway / koi bhi docker host)
#
# ⚠️  ROOT-CAUSE FIX (2026-09-21): PEHLE ye Dockerfile ek PREBUILT binary
#     (`gold-md-cgofree`) COPY karta tha — `go build` NAHI. Iska matlab:
#     source me koi bhi fix karne ke baad, agar binary dobara build na kiya
#     jaye, to Render PURANA binary deploy karta rehta tha. Yahi wajah thi ki
#     "session reconnect delete" + "SUPERVISOR_ENABLED fix" source me hone ke
#     bawajood live bot me asar nahi kar rahe the (restart/reconnect loop
#     chalta rehta tha).
#
#     AB: MULTI-STAGE SOURCE BUILD. Stage-1 golang:1.26-bookworm me source se
#     binary compile hoti hai (vendored deps, CGO_ENABLED=0, modernc.org/sqlite
#     — CGO-free). Stage-2 debian:bookworm-slim runtime. Isse SOURCE == DEPLOYED
#     hamesha guaranteed. Koi stale-binary drift kabhi nahi.
#
# Full command parity: ffmpeg (play/sticker/tomp3), python3+Pillow (pdf),
# libreoffice (document->pdf), poppler (pdf text), jpegoptim/pngquant (compress).
# Bot missing tools ko self-heal karta hai — kuch miss ho to graceful error,
# crash nahi.
#
# PORT: ENV 11221 default; platform env (Render / Back4app / Railway) hamesha
#       override karega — bot ka dotenv loader real env vars ko priority deta
#       hai (verified dotenv.go).
# ============================================================================

# ── Stage 1: builder (source -> static CGO-free binary) ─────────────────────
FROM golang:1.26-bookworm AS builder
WORKDIR /build
# Poora source tree copy (src + gold-cmds + vendor + go.mod/go.sum). Vendor dir
# repo me committed hai (154MB) — -mod=vendor se koi network fetch nahi,
# whatsmeow vendor patches (pair-code.go) automatically apply hote hain.
# .dockerignore .git / nexstore / logs / binaries exclude karta hai.
COPY . .
# build-stamp: 20260921-060000 source-build (stale-binary root-cause fix)
RUN echo "GOLD-MD build-stamp 20260921-060000 source-build"
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o /build/gold-md ./src

# ── Stage 2: runtime ────────────────────────────────────────────────────────
FROM debian:bookworm-slim

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

# ngrok agent — optional runtime tool (NGROK_AUTHTOKEN + NGROK_DOMAIN env vars
# container runtime pe set karo, image me bake NAHI karna).
# NON-FATAL: Render builder egress proxy equinox.io CDN ko block kar sakta hai
# (curl exit 107). Agar download fail ho to ngrok SKIP — build/pass kharab
# NAHI hoga. Bot ngrok ke bina bilkul theek chalta hai (sirf tunnel off).
RUN set -eux; \
    if curl -fsSL --retry 3 --retry-delay 2 --connect-timeout 20 \
            https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz \
            -o /tmp/ngrok.tgz 2>/dev/null; then \
        tar xzf /tmp/ngrok.tgz -C /usr/local/bin; \
        rm -f /tmp/ngrok.tgz; \
        chmod +x /usr/local/bin/ngrok; \
        /usr/local/bin/ngrok --version || true; \
    elif wget -q --tries=3 --timeout=20 -O /tmp/ngrok.tgz \
            https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz \
            && [ -s /tmp/ngrok.tgz ]; then \
        tar xzf /tmp/ngrok.tgz -C /usr/local/bin; \
        rm -f /tmp/ngrok.tgz; \
        chmod +x /usr/local/bin/ngrok; \
        /usr/local/bin/ngrok --version || true; \
    else \
        echo "WARN: ngrok download FAILED (proxy/CDN block) — skipping ngrok install, build continue"; \
    fi

ENV PORT=11221
# ── SUPERVISOR_ENABLED=1 (owner order — restart/reconnect loop FIX) ──
# start.sh khud supervisor hai (bot exit pe 5s baad single fresh process
# uthata hai). Is env ke bina:
#   1. memoryWatchdog cgroup memory.current (page-cache samet) dekhta hai
#      → Render 512MB container pe 450MB threshold bar bar cross hota hai
#      → baar baar self-restart.
#   2. gracefulSelfRestart fork+exec karta hai (Setsid child) → start.sh
#      ka wait-loop bhi 5s baad doosra process uthata hai → 2 processes
#      same WhatsApp session pe → "stream replaced" kick-kick loop.
# SUPERVISOR_ENABLED=1 pe: (a) RAM sirf bot ki apni RSS se naapi jati hai,
# (b) restart pe clean exit hota hai → start.sh akela ek process uthata hai.
ENV SUPERVISOR_ENABLED=1
EXPOSE 2081
CMD ["./start.sh"]
