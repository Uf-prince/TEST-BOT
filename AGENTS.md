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
- Voice (MP3) siblings: `.botvoice` is bot-wide, and one hidden command per
  menu (`.menuvoice`, `.logovoice`, `.aimenuvoice`, `.fontvoice`, `.gamevoice`,
  ...). The voice plays right AFTER the menu / `.alive` header is sent (see
  `sendMenuHeader` -> `sendMenuVoice`). Resolution: per-menu -> bot-wide ->
  built-in default (`goldcmds.DefaultMenuVoiceURL`). A per-menu `.xvoice reset`
  stores the `off` sentinel and silences JUST that menu; `.botvoice reset`
  silences all. Redis fields: `botvoice`, `menumedia:<key>:voice`.

- Category menu shortcuts are resolved BEFORE the prefix-match fallback
  (`categoryMenuShortcut` in handler.go). The fallback is greedy, so `.ai` and
  `.group` were previously stolen by `aiimage` / `groupban` and those category
  menus never opened. An exact registered command always wins.

## AI Mode (`.aimode`) — prefixless-command gate
- The AI Mode wake-word (`aimodeprefix`) FALLS BACK to `aimDefaultPrefixWord`
  (`"AI"`) when unset — same as pair.js `AIMODE_DEFAULT_PREFIX_WORD`. Never
  treat an empty wake-word as "read every plain message": that let bare command
  names dispatch without the bot prefix. The resolver only fires on messages
  that start with the wake-word (`AIModeTryHandle`, `gold-cmds/aimode.go`).
- The stored wake-word is resolved through `normalizeAimWakeWord` before the
  gate regex is built: `""`, whitespace-only, `"null"` and the missing sentinel
  `"\x00"` (the same value the prefix key once leaked from `dcPopulateRAM`) all
  mean "not set" → default. Reading the raw value built a gate that could never
  match and silently disabled AI Mode.
- `.aimode` texts live in `aimode.go` (`aimToggleText`, `aimStatusText`,
  `aimPrefixInfoText`, `aimPrefixSetText`, `aimGuideText`, `aimUsageText`).
  Follow the GOLD-MD design: bold, emoji marks, value/command brackets, and
  `LABEL :` description rows. Owner rejection uses the bot-wide owner-only
  notice.
- The connected/startup card carries the live AI Mode state (`AIModeEnabledFor`
  / `AIModeWakeWordFor`, read from Redis via `AIModeAttachSettingReader`, wired
  in `src/main.go`). Example wake-word lines show ONLY when AI Mode is ON.
- `.aimode on/off/prefix` re-sends the card through the unthrottled
  `NotifyConnectedCard` bridge hook (pair.js `BilalSendConnectedNotice` parity).

## Bot language (`.botlanguage`) — replies only, never command names
- One command sets the language the bot REPLIES in: `gold-cmds/botlangcmd.go`.
  Guide is ENGLISH on purpose (default user is English); the language list is
  `trtLangs` in `gold-cmds/trt.go` (130+ Google NMT languages), each shown with
  its English name.
- Storage: Redis `settings:<botJID>.botlanguage` = a language code ("ur","hi",
  ...). Empty/`en` = no translation. Use `Get/SetBotLanguageSetting` on the
  SessionBridge (`src/commands_loader.go`).
- The translation layer lives on `*Session` in `src/handler.go`: `botLanguage()`
  (cached), `translateOut()` (8s timeout, falls back to English on error) and
  `replyText()` = translate → skin → footer. `Reply`, `SendTextWithID`,
  `sendSimple` and the group notices use `replyText`; media captions translate
  in `withCaptionFooter`.
- OWNER ORDER: COMMAND NAMES ARE NEVER TRANSLATED. `.ping`, `.menu` stay English
  for every user; only the bot's own words change language. Command dispatch is
  untouched by the language layer.

## Style commands — only `.botstyle`
- OWNER ORDER: the per-menu style commands (`menustyle`, `logostyle`,
  `fontstyle`, `gamestyle`, `equalizerstyle`, `aimenustyle`, `botmenustyle`,
  `botstylestyle`) are REMOVED. `gold-cmds/menustylecmd.go` no longer registers
  anything; `.botstyle` (`gold-cmds/botskincmd.go`) is the ONLY style switch —
  it writes both `botstyle` (text skin) and `botmenustyle` (menu chrome).
- `menuStyleFor` (`src/manager.go`) resolves every menu from `botstyle` now;
  legacy `menustyle:<key>` / `botmenustyle` values are still honoured.

