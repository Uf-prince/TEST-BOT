package main

// ═══════════════════════════════════════════════════════════════════════════
//   ASSET / VOICE STORJ BACKUP + RESTORE
//
//   Owner requirement: every saved .addimg/.addvideo/.addsticker/.addtext/
//   .addcircle asset and every .addvoice clip must live in the durable store
//   as well as on disk, so a RAM-disk wipe / restart can rebuild the local
//   files instead of losing them.
//
//   Model (write-through + lazy read-through):
//     • save  → disk first (source of truth), then one Storj PUT (background)
//     • read  → if the local file is gone, pull it from Storj and rehydrate disk
//     • list  → if the local index + folder are empty, rebuild from Storj keys
//     • delete→ remove the local files AND the Storj object
//
//   Namespaces (one per kind so list is a cheap prefix scan):
//     goldmd:assets:<kind>/<botJID>/<name>.json
//     goldmd:voices/<botJID>/<name>.json
//
//   Payload: {"d":<base64>,"m":<mime>,"x":<meta>}
// ═══════════════════════════════════════════════════════════════════════════

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// assetStorjTimeout bounds every backup / restore round-trip so a slow store
// can never hang a command or the connect path.
const assetStorjTimeout = 30 * time.Second

// assetBackupPayload is the JSON body stored for one asset or voice.
type assetBackupPayload struct {
	Data []byte `json:"d"`
	Mime string `json:"m"`
	Meta string `json:"x,omitempty"`
}

// assetStorjNS maps a storage kind to its Storj namespace.
func assetStorjNS(kind string) string {
	if kind == "voice" {
		return "goldmd:voices"
	}
	return "goldmd:assets:" + kind
}

// assetStorjID is the object id: <botJID>/<sanitised name>.
func assetStorjID(botJID, safeName string) string {
	return botJID + "/" + safeName
}

// assetStorjPut backs up one asset/voice payload. Best-effort: returns false
// when the store is disabled or the write fails (caller keeps the disk copy).
func assetStorjPut(ns, id string, data []byte, mime, meta string) bool {
	if !storj.Ready() || ns == "" || id == "" || len(data) == 0 {
		return false
	}
	body, err := json.Marshal(assetBackupPayload{Data: data, Mime: mime, Meta: meta})
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), assetStorjTimeout)
	defer cancel()
	if err := storj.PutJSONNS(ctx, ns, id, body); err != nil {
		WarnLog("ASSET-BACKUP: put %s/%s failed: %v", ns, id, err)
		return false
	}
	return true
}

// assetStorjGet fetches a backed-up payload. Returns ok=false when missing.
func assetStorjGet(ns, id string) (data []byte, mime, meta string, ok bool) {
	if !storj.Ready() || ns == "" || id == "" {
		return nil, "", "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), assetStorjTimeout)
	defer cancel()
	body, found, err := storj.GetJSONNS(ctx, ns, id)
	if err != nil || !found || len(body) == 0 {
		return nil, "", "", false
	}
	var p assetBackupPayload
	if jerr := json.Unmarshal(body, &p); jerr != nil || len(p.Data) == 0 {
		return nil, "", "", false
	}
	return p.Data, p.Mime, p.Meta, true
}

// assetStorjDelete removes a backup. Idempotent; silent on failure.
func assetStorjDelete(ns, id string) {
	if !storj.Ready() || ns == "" || id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), assetStorjTimeout)
	defer cancel()
	if err := storj.DeleteJSONNS(ctx, ns, id); err != nil {
		WarnLog("ASSET-BACKUP: delete %s/%s failed: %v", ns, id, err)
	}
}

// assetStorjListNames returns every saved name for a bot under one namespace.
// The prefix is <botJID>/, so ids come back as "<botJID>/<name>" and the
// leading segment is stripped before returning.
func assetStorjListNames(ns, botJID string) []string {
	if !storj.Ready() || ns == "" || botJID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ids, err := storj.ListJSONNSKeys(ctx, ns)
	if err != nil {
		WarnLog("ASSET-BACKUP: list %s failed: %v", ns, err)
		return nil
	}
	prefix := botJID + "/"
	var names []string
	for _, id := range ids {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		name := strings.TrimPrefix(id, prefix)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// assetStorjRestoreInto pulls one asset from the store and writes it back to
// disk (payload + .mime + .meta), rebuilding the Redis name index. Returns
// true when a local file now exists.
func (b *bridge) assetStorjRestoreInto(kind, name string) bool {
	data, mime, meta, ok := assetStorjGet(assetStorjNS(kind), assetStorjID(b.s.JID, sanitiseAssetName(name)))
	if !ok || len(data) == 0 {
		return false
	}
	return b.writeAssetLocal(kind, name, data, mime, meta, false)
}

// voiceStorjRestoreInto is assetStorjRestoreInto for saved voices.
func (b *bridge) voiceStorjRestoreInto(name string) bool {
	data, mime, _, ok := assetStorjGet(assetStorjNS("voice"), assetStorjID(b.s.JID, strings.ToLower(strings.TrimSpace(name))))
	if !ok || len(data) == 0 {
		return false
	}
	return b.saveVoiceLocal(name, data, mime, false)
}

// restoreAssetsFromStorj (re)builds the local name index for every kind of one
// bot from the store. Only names with no local file are downloaded, so a warm
// disk costs just a few list calls. Runs in the background at connect time.
func (b *bridge) restoreAssetsFromStorj() {
	if !storj.Ready() {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			ErrLog("[%s] restoreAssetsFromStorj panic: %v", b.s.JID, r)
		}
	}()

	kinds := []string{"img", "video", "sticker", "text", "circle"}
	restored := 0
	for _, kind := range kinds {
		for _, name := range assetStorjListNames(assetStorjNS(kind), b.s.JID) {
			// Index the name even when the file is already local, so .add<kind>
			// list works after a Redis/disk wipe.
			if b.s.Manager.Redis != nil {
				_ = b.s.Manager.Redis.setAdd("goldmd:"+b.s.JID+":"+kind, name)
			}
			if _, _, ok := b.GetCustomAsset(kind, name); ok {
				continue
			}
			if b.assetStorjRestoreInto(kind, name) {
				restored++
			}
		}
	}
	for _, name := range assetStorjListNames(assetStorjNS("voice"), b.s.JID) {
		if b.s.Manager.Redis != nil {
			_ = b.s.Manager.Redis.setAdd("goldmd:"+b.s.JID+":voices", name)
		}
		if b.voiceStorjRestoreInto(name) {
			restored++
		}
	}
	if restored > 0 {
		OkLog("[%s] ASSET-BACKUP: restored %d asset(s)/voice(s) from Storj", b.s.JID, restored)
	}
}
