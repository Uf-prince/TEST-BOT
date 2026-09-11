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
COPY gold-md-cgofree ./gold-md
COPY servers.json .
# .env REMOVED — all credentials are hardcoded in source (storj.go / config.go)
RUN chmod +x ./gold-md && mkdir -p nexstore/pairing

ENV PORT=11224
EXPOSE 2081
CMD ["./gold-md"]
