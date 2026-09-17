# ============================================================================
# GOLD-MD — ROOT Dockerfile (Back4app Containers / koi bhi docker host)
# SLIM: prebuilt CGO-free binary COPY hota hai — build sirf 1-2 min,
# weak shared builders (Back4app free 0.25 CPU) pe bhi safe. Go compile nahi.
#
# Binary: gold-md-cgofree (Linux amd64, CGO_ENABLED=0, modernc.org/sqlite,
#         whatsmeow vendor patches included) — repo me committed hai.
#         Dubara build karna ho (source change ke baad):
#           CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o gold-md-cgofree .
#
# Full command parity: ffmpeg (play/sticker/tomp3), python3+Pillow (pdf),
# libreoffice (document->pdf), poppler (pdf text), jpegoptim/pngquant (compress).
# Bot missing tools ko self-heal karta hai — kuch miss ho to graceful error,
# crash nahi.
#
# PORT: ENV 2081 default; platform env (Back4app app settings / Railway /
#       Render) hamesha override karega — bot ka dotenv loader real env
#       vars ko priority deta hai (verified dotenv.go).
#
# NOTE: Purana source-build Dockerfile (GHCR golang:1.26 builder wala)
#       Dockerfile.ghcr me preserved hai.
# ============================================================================
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates libc6 ffmpeg python3 python3-pil \
        jpegoptim pngquant poppler-utils \
        libreoffice-writer libreoffice-calc libreoffice-impress \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
# build-stamp: 20260917-080224 tt render fix (tikwm fallback + jina brave search)
# ^ ye line har fresh rebuild pe update hoti hai — Docker layer cache invalidate
#   karti hai taake Render purani cached image use na kare.
RUN echo "GOLD-MD build-stamp 20260917-080224"
COPY gold-md-cgofree ./gold-md
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

ENV PORT=11224
EXPOSE 2081
CMD ["./start.sh"]
