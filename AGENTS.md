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
- Auto-send: a bare asset name triggers `applyAssetTrigger` in
  `src/handler.go`. One name may hold several kinds at once (the owner can save
  an image, video, sticker, circle AND text all as `umar`); `AssetKindsToSend`
  returns EVERY saved kind and the trigger sends each as its own message in
  `goldcmds.AssetTriggerOrder` (media before text). Never collapse this to a
  single "best" kind — that is the bug where a sticker got shadowed by an image
  or an image by a video. Voices are delivered separately by
  `applyVoiceTrigger`, so `voice` is not part of `AssetTriggerOrder`.
- `.addtext` auto-send is edit-style: `sendAssetTextEdited` sends the saved text
  then edits that same message to the same text after `assetEditDelay` (1s),
  mirroring `arRunReplyJob` in `gold-cmds/autoreply.go` so the message carries
  the "Edited" mark. Do not change it to a plain reply.
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
  (`whatsappifyVideo`) or WhatsApp errors on playback. This applies to MENU
  headers too: `.botvideo` file/URL uploads and per-menu videos all pass through
  `goldcmds.BotVideoNormalizeBytes` (ffmpeg h264, even dims, faststart) before
  being stored — the fix for "can't play this video" on menu clips.

## Per-menu media (`.menupic` / `.menuvideo` + per-menu variants)

- Every menu can carry its OWN header picture and video, set by 34 generated
  commands in `gold-cmds/menumedia.go` (`menuMediaSpecs`): `menupic`/`menuvideo`
  (main menu, visible for discovery) plus hidden per-menu pairs such as
  `logopic`/`logovideo`, `fontpic`, `gamevideo`, `converterpic`, `toolspic`,
  `aimenupic`/`aimenuvideo`, `corepic`, `groupmenupic`, `greactionvideo`, etc.
  All are `OwnerOnly`. Their no-arg guides list every command.
- Storage: Redis settings `menumedia:<key>` (picture) and `menumedia:<key>:video`
  (video). `menuMediaSettingKey` is the single source of truth;
  `goldcmds.MenuMediaSettingKey` is the exported form. Keys are menu slugs from
  `menuCategorySlugs` (`menu`, `logo`, `alive`, `ai`, `converter`, `tools`, ...).
- Resolution order (in `src/manager.go`): per-menu override → bot-wide
  `.botpic`/`.botvideo` → default image / text-only fallback. A per-menu PIC
  also suppresses the bot-wide VIDEO for that same menu — otherwise `.logopic`
  looks like a no-op while `.botvideo` keeps hijacking the list. An explicit
  per-menu VIDEO outranks the per-menu pic.
  `sendMenuHeader(info, key, caption)` renders it, with `menuPicURL(key)` /
  `menuVideoURL(key)` resolving it. An empty key means "no per-menu override".
  Wire every menu through `sendMenuHeader` — never call `botPicURL` directly.
- Confirmation cards name the exact menu (`*LOGO MENU PIC UPDATED*`) and point
  the owner at the command that OPENS it (`*FOR TEST TYPE ❰ .logo ❱*`, `.ai`,
  `.game`). Never the media command itself (`.logopic` only SETS the picture)
  and never a generic `.alive` / `.menu` hint.
- Category menu shortcuts are resolved BEFORE the prefix-match fallback
  (`categoryMenuShortcut` in handler.go). The fallback is greedy, so `.ai` and
  `.group` were previously stolen by `aiimage` / `groupban` and those category
  menus never opened. An exact registered command always wins.
