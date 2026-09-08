FROM golang:1.26-bookworm AS builder
WORKDIR /build
COPY . .
# Use vendored deps (-mod=vendor) so the PR #1234 passkey patch in
# vendor/go.mau.fi/whatsmeow/pair-code.go travels with the repo and
# applies automatically on every fresh deploy. No network module fetch.
RUN CGO_ENABLED=1 go build -mod=vendor -o gold-md .

# ── runtime ───────────────────────────────────────────────────────
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates libc6 ffmpeg python3 python3-pil \
    libreoffice-writer libreoffice-calc libreoffice-impress \
    poppler-utils && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /build/gold-md .
COPY --from=builder /build/servers.json .
# .env shipped as a fallback for hosts that skip env injection;
# creds are ALSO hardcoded in storj.go / config.go / upstash.go.
COPY --from=builder /build/.env .
RUN mkdir -p nexstore/pairing
EXPOSE 2081
CMD ["./gold-md"]
# build-stamp: 20260908-145029 volume auto-commit fix
