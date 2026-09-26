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

## Reply translation (.botlanguage) — token safety

- Every reply passes through `goldcmds.TranslatePreservingCommandTokens`
  (`gold-cmds/menutrans.go`) from `src/handler.go`. A line advertising a command
  is split at its last token: the bracketed token stays verbatim while the
  description after it is translated. Never send a menu line whole to a
  translator — Google returns just the bracket and every command name vanishes.
- Token detection is registry-based (`clKnownCommandNames`), so prose like "e.g."
  or "photo.jpg" is not mistaken for a command. Blank lines are excluded from the
  batch because they make translators collapse the run, which trips the
  line-count guard and reverts the whole reply to English.
- `trtTranslate` (`gold-cmds/trt.go`) tries Google's free endpoint first and falls
  back to MyMemory (key-less). Google throttles by returning a ONE-CHARACTER
  "translation" instead of an error, so `trtPlausible` rejects that and triggers
  the fallback. Keep that check — without it a throttled reply silently replaces
  a menu line with a single letter.
- `clBuildAsync` MERGES the name map instead of replacing it, so a throttled chunk
  cannot wipe aliases that already work. Maps carrying `com.*` rows are rebuilt
  once to drop that legacy junk (`clMapHasLegacyJunk`).

## Translator (.trt) — language catalog and Pakistani script

- `trtLangs` (`gold-cmds/trt.go`) is the whole language catalog: **252 codes**.
  Google's own live endpoint (`https://translate.google.com/translate_a/l?client=gtx&alpha=true&hl=en`)
  reports 249; the extra three (`fil`, `he`, `jv`) are legacy codes Google no
  longer lists but the translate endpoint still accepts. Do not regenerate the
  table from the Cloud docs page — that page mixes in region variants
  (`en-GB`, `es-MX`, `ar-SA`) that the free endpoint rejects; always diff
  against the live endpoint and then verify each new code actually translates.
- Every code in the table was verified to return a real translation through the
  same endpoint the bot uses (`clients5.google.com/translate_a/t`). A code that
  returns HTTP error is not supported, and one that echoes the English input
  unchanged is a fake pass (Kashmiri `kas` does exactly this) — never add those.
- Pakistani Punjab is **Shahmukhi, not Gurmukhi**. Google exposes Punjabi twice:
  `pa` = Gurmukhi (Indian script), `pa-Arab` = Shahmukhi (Pakistani script). All
  Punjab-Pakistan cities and dialects (Lahore, Multan, Rawalpindi, Saraiki,
  Hindko, Pothwari, Jhangvi, ...) resolve to `pa-Arab`. Only `punjabi (india)`,
  Amritsar and Chandigarh use `pa`. Keep it that way or Pakistani users get
  Indian-script Punjabi back.
- Languages Google genuinely does not support (Saraiki, Hindko, Brahui, Western
  Punjabi `pnb`, Khowar, Shina) resolve to the nearest supported code rather than
  being added — adding them would produce an HTTP error at reply time.
- Mistral (the autoreply model) is **not** a translation engine. Asked to
  translate into Saraiki/Hindko it hallucinates (`سَبحانِ خُدا`), and for
  Shahmukhi Punjabi it returns Gurmukhi. It also self-reports "over 100
  languages" and admits it does not know all ~7,000. Keep translation on the
  Google/MyMemory path; do not route `.trt` through the AI pool.
- `TestNewLanguagesLive` is opt-in: `GOLDMD_LIVE_TEST=1 go test -run TestNewLanguagesLive ./gold-cmds/`.

## Public tunnel

- `nohup cloudflared tunnel --url http://localhost:48467 > /tmp/cf_tunnel.log 2>&1 &`
  then read the URL from `/tmp/cf_tunnel.log` (changes on every restart).
- `GITHUB_TOKEN` has no write access to this repo; pushing `main` needs the
  user's PAT in the remote URL.

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

## Bot language (`.botlanguage`) — replies + localized command aliases
- One command sets the language the bot REPLIES in: `gold-cmds/botlangcmd.go`.
  Guide is ENGLISH on purpose (default user is English); the language list is
  `trtLangs` in `gold-cmds/trt.go` (130+ Google NMT languages), each shown with
  its English name. `.botlanguage commands` lists the localized command names
  that currently work.
- Storage: Redis `settings:<botJID>.botlanguage` = a language code ("ur","hi",
  ...). Empty/`en` = no translation. Use `Get/SetBotLanguageSetting` on the
  SessionBridge (`src/commands_loader.go`).
- REGIONS & DIALECTS: `trtRegionAliases` (`gold-cmds/trt.go`) maps a country,
  city, town or dialect name (Lahore, Karachi, Saraiki, Hindko, Nairobi, Cairo,
  ...) to the nearest language Google actually supports. Entering a place name
  is how "har sheher / har gaon" is covered. Every alias value MUST be a code
  present in `trtLangs` (enforced by a test).
- The translation layer lives on `*Session` in `src/handler.go`: `botLanguage()`
  (cached), `translateOut()` (8s timeout, falls back to English on error) and
  `replyText()` = translate → skin → footer. `Reply`, `SendTextWithID`,
  `sendSimple` and the group notices use `replyText`; media captions translate
  in `withCaptionFooter`.
- LOCALIZED COMMAND ALIASES (`gold-cmds/cmdlocalize.go`): when a language is
  set, the bot prepares a localized name for every visible command by
  translating all names in ONE call (chunked at 150 names — the endpoint
  returns HTTP 400 for the full ~2700-name payload) and caching the map in
  Redis `settings:<botJID>.cmdlocalize`. `CmdLocalizeResolve` maps a typed
  localized token back to its English command during dispatch
  (`src/handler.go`, right after `CmdNameResolve`). English names ALWAYS keep
  working; the localized name is only an alias, so owner-only / bancmd /
  cmdowner / mode checks run on the canonical command unchanged.
- Unicode tokens: `cmdRegexAny` (`src/handler.go`) parses a non-ASCII command
  token (`.مینو`, `.मेनू`) after the ASCII and skin parses fail. Unknown
  non-ASCII tokens match no command and stay silent, as before.
- OWNER ORDER: command NAMES are never translated in dispatch terms — the
  registry stays ASCII English. Localized names are aliases in Redis only.

## Style commands — only `.botstyle`
- OWNER ORDER: the per-menu style commands (`menustyle`, `logostyle`,
  `fontstyle`, `gamestyle`, `equalizerstyle`, `aimenustyle`, `botmenustyle`,
  `botstylestyle`) are REMOVED. `gold-cmds/menustylecmd.go` no longer registers
  anything; `.botstyle` (`gold-cmds/botskincmd.go`) is the ONLY style switch —
  it writes both `botstyle` (text skin) and `botmenustyle` (menu chrome).
- `menuStyleFor` (`src/manager.go`) resolves every menu from `botstyle` now;
  legacy `menustyle:<key>` / `botmenustyle` values are still honoured.

