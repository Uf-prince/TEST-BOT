# AGENTS.md — GOLD-MD bot (Uf-prince/TEST-BOT)

Repo: `/workspace/testbot` (origin `Uf-prince/TEST-BOT`, branch `main`).
Workflow: commit directly to `main`, no PR. Replies to users are Hinglish /
Roman Urdu with `*BOLD UPPERCASE*` formatting.

## Layout & conventions

- `gold-cmds/` — command implementations. Each `init()` calls
  `Register(Command{...})`. Visible commands get `Category` + `Desc`; aliases
  are `Hidden: true`. `OwnerOnly: true` for owner commands.
- `src/` — runtime. `bridge` implements `SessionBridge` (declared in
  `gold-cmds/api.go`) over a `Session`. Adding a bridge method means updating
  BOTH the interface in `gold-cmds/api.go` and `src/`.
- Tests live next to the code (`*_test.go`). Heavy/ffmpeg tests skip when
  ffmpeg is unavailable.
- Command/menu tests intentionally pin historical counts (e.g.
  `convcount_test.go` expects exactly 100 visible CONVERTER commands). Adding a
  visible command breaks them — update the pinned numbers in the same change if
  the count test is meant to track reality.

## Known pre-existing test failures (not caused by new work)

Clean tree still fails: `TestConverterCategoryCount`, `TestFunPackRegistered`,
`TestFunPackAliasesHidden`, `TestLyricsAutoDetect`, `TestATTPBuildSticker`,
`TestSearchPick`, `TestCorrectLinkApkSearchApkLink`,
`TestFleetServerNumberForSIDTable`, `TestGuardCompressVideoBig`,
`TestGuardPolicyTiers`, `TestGuardForceKnob`, `TestGuardOverLimitDocBlock`,
`TestSlotsUsedCountsInFlightReservations`, `TestSlotsUsedConcurrentHammer`.
Don't chase these unless asked.

## Toolchain

- Go is at `/workspace/tools/go/bin` (not on PATH by default). ffmpeg/ffprobe
  are bundled in `nexstore/ffmpeg`.
  ```
  export PATH=/workspace/testbot/nexstore/ffmpeg:/workspace/tools/go/bin:$PATH
  CGO_ENABLED=0 go build -mod=vendor -o gold-md ./src
  CGO_ENABLED=0 go test -mod=vendor ./gold-cmds/ ./src/
  ```
- Local run (panel + sessions): `PORT=48467 GOLDMD_PANEL_ENABLED=1 nohup ./gold-md > /tmp/bot_gold.log 2>&1 &`
  Session status: `curl -s localhost:48467/sessions`.

## Media features

- Named asset store (`SaveCustomAsset`/`GetCustomAsset`/`ListCustomAssets`/
  `DeleteCustomAsset` + `*Meta` variants) in `src/handlers_assets.go`, used by
  `.addimg/.addvideo/.addsticker/.addtext/.addcircle` (`gold-cmds/assets.go`).
  Files: `<DataDir>/assets/<jid>/<kind>/<name>.bin` + `.mime` + `.meta`.
  Names are sanitised to `[a-z0-9_-]`; never bypass `assetPath`.
- Auto-send: bare asset name triggers `applyAssetTrigger` in
  `src/handler.go` (parallel to `applyVoiceTrigger`). One name may be saved
  under several kinds; `goldcmds.SelectNewestAssetKind` picks the most recently
  saved match (ties fall back to `goldcmds.AssetTriggerOrder` — media before
  text), so a fresh `.addsticker` is not shadowed by an older photo.
- Durable backup: every asset and voice is mirrored to Storj
  (`src/assets_storj.go`, write-through on save, read-through on cache miss).
  Namespaces `goldmd:assets:<kind>/<jid>/<name>` and `goldmd:voices/<jid>/<name>`;
  `restoreAssetsFromStorj()` runs at connect to rebuild a wiped disk + index.
  Keep `assetStorjNS`/`sanitiseAssetName` as the single source of truth for keys.
- WhatsApp circle videos are `Message.PtvMessage` (field 66) holding a SQUARE
  `VideoMessage`. `.circle` centre-crops + re-encodes with ffmpeg
  (`gold-cmds/circle.go`). Always keep the square crop — a non-square clip is
  not rendered as a circle. ContextInfo may sit on `PtvMessage` itself, not
  just `ExtendedTextMessage` — always resolve quotes via
  `extractContextInfoFromMsg`/`extractMediaMessage` in `src/manager.go`.
- Before sending video externally, normalise to h264+aac+faststart
  (`whatsappifyVideo`) or WhatsApp errors on playback.