# GOLD-MD: Port commands from pair.js to Go (whatsmeow)

## Group 1: Premium commands (antilinkprem, antibotprem, antibadprem)
- [x] Create gold-cmds/premium.go (add/del/list, 👑→🔰, owner OR group-admin)
- [x] Verify bridge methods (PremiumAdd/Remove/List/IsMember) exist in api.go + commands_loader.go

## Group 2: Bot block / unblock / banlist
- [x] Create gold-cmds/botblock.go (botblock/botunblock/banlist, owner-only)

## Group 2: autoread
- [x] Create gold-cmds/autoread.go (off/inbox/groups/all modes, owner-only)
- [x] Add autoread hook in handler.go (go s.applyAutoRead(info))
- [x] Create applyAutoRead function in handler.go (MarkRead based on mode)

## Group 2: antistatus
- [x] Create gold-cmds/antistatus.go (on/off/action/warn/delete/kick/reset, group+owner-only)

## Group 2: bangc / unbangc
- [x] Create gold-cmds/bangc.go (lock/unlock entire group, owner+group-only)
- [x] Add bangc enforcement in handler.go (block commands if group locked)

## Group 2: bangcuser / unbangcuser
- [x] Create gold-cmds/bangcuser.go (per-group per-user ban, admin/owner-only, list subcommand)
- [x] Add bangcuser enforcement in handler.go (block commands if user banned in group)

## Bot-wide ban enforcement
- [x] Add bot-wide ban check in handler.go (BotBanIsBanned before command runs)

## Build & Deploy
- [x] Verify all with whatsmeow latest WhatsApp Go API (build succeeded)
- [x] Rebuild binary (CGO enabled, Go 1.26) — BUILD SUCCESS
- [x] Restart localhost on port 11236 — RUNNING, session online
- [x] Push fresh files to GitHub — PUSHED (commit a05855d)
