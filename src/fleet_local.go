package main

// ════════════════════════════════════════════════════════════════════════════
//   GOLD-MD — LOCAL FLEET REGISTRY  (owner order, 2026-09-21)
//
//   OWNER ORDER (Roman Urdu, verbatim intent):
//     "Agar yh session ki info claim servers ki info hmesha local file me se
//      read honi chahye. Agar folder na mile to setobara storage se ek bar
//      load kr lo fir bara bar nai. Ek guard bithao jo us folder ke sessions
//      ko online check krta rahe live har waqt — jese hi offline ho fleet
//      walo ko info kr de k ye session off hai, iska claim dusre server ko
//      do, wo fir claim kre ek bar."
//
//   ── KYA HAI YE ──────────────────────────────────────────────────────────
//   Ek LOCAL JSON file (nexstore/fleetlocal/registry.json) jo har session ki
//   info + uska claim-holder server + har server ka URL/heartbeat rakhti hai.
//
//   READ RULE (owner: "hamesha local file se read"):
//     • fleetClaimHolders()  → pehle local registry
//     • fleetHeartbeatMap()  → pehle local registry
//     • fleetServerURL()     → pehle local registry
//     • local me na mile      → Storj fallback (ek baar) + local populate
//
//   LOAD RULE (owner: "folder na mile to Storj se ek bar load, fir bar bar nai"):
//     • Boot pe: agar registry.json mojood hai → seedha disk se (0 network).
//     • Boot pe: agar registry.json GAYAB → Storj se EK BAAR bulk-load,
//       phir "_loaded" marker likh do → dobara kabhi full-load nahi.
//     • Continuous: sirf MERGE-refresh (60s) — Storj se sirf BADLI hui
//       entries aati hain, local kabhi wipe nahi hota, apni fresh claim
//       kabhi overwrite nahi hoti. (Isliye "bar bar" full-load nahi hota.)
//
//   WRITE RULE (write-through): har claim/heartbeat/online write local file
//   me turant likha jata hai (atomic rename) — taake read hamesha local se ho.
//
//   OFFLINE NOTIFY (owner: "jese hi offline ho fleet walo ko info kr de"):
//     flNotifyOffline(jid) → local online=false + apna claim release (Storj
//     HDEL + local remove) + fail-marker (doosre server ko pata chale) →
//     watchdog/resurrector us JID ko FREE server pe dispatch karta hai →
//     wo server EK BAAR claim karta hai.
//
//   BANDWIDTH: reads 100% local (0 network). Sirf merge-refresh 60s pe 3
//   chhoti keys (servers hash + sessions set + changed claims) padhta hai —
//   pehle se kam ya barabar. Bot speed pe 0% asar (sab background).
// ════════════════════════════════════════════════════════════════════════════

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── config ──────────────────────────────────────────────────────────────────
var flEnabled = envBool("GOLDMD_FLEET_LOCAL", true)

var (
	flReady bool
	flDir   string
	flPath  string
	flMu    sync.Mutex
	flReg   = &flRegistry{Sessions: map[string]*flSession{}, Servers: map[string]*flServer{}}
)

// flSession: ek session ki local info (claim holder + live status).
type flSession struct {
	JID       string `json:"jid"`
	ServerID  string `json:"server_id"`  // claim holder (kaun chala raha hai)
	ServerURL string `json:"server_url"` // holder ka public URL
	ClaimTS   int64  `json:"claim_ts"`   // claim timestamp (unix)
	Online    bool   `json:"online"`     // last known live status
	Updated   int64  `json:"updated"`    // last local update (unix)
}

// flServer: ek server ki local info (heartbeat + URL + capacity).
type flServer struct {
	SID      string `json:"sid"`
	URL      string `json:"url"`
	TS       int64  `json:"ts"`       // heartbeat ts
	Sessions int    `json:"sessions"` // -1 = unknown
	Max      int    `json:"max"`      // -1 = unknown
}

// flRegistry: poori local file ka shape.
type flRegistry struct {
	Self     string                `json:"self"`
	SelfURL  string                `json:"self_url"`
	Updated  int64                 `json:"updated"`
	Sessions map[string]*flSession `json:"sessions"`
	Servers  map[string]*flServer  `json:"servers"`
}

// ── boot ────────────────────────────────────────────────────────────────────

// flInit: boot pe ek baar. Disk se load; disk gayab ho to Storj se EK BAAR
// bulk-load (marker ke saath — dobara nahi). fleetInit se call hota hai.
func flInit() {
	if !flEnabled {
		InfoLog("FLEET-LOCAL: disabled (GOLDMD_FLEET_LOCAL=0)")
		return
	}
	flDir = envOr("GOLDMD_FLEET_LOCAL_DIR",
		filepath.Join(envOr("GOLDMD_DATA_DIR", "nexstore"), "fleetlocal"))
	if err := os.MkdirAll(flDir, 0o755); err != nil {
		ErrLog("FLEET-LOCAL: mkdir %s failed: %v — disabled", flDir, err)
		flEnabled = false
		return
	}
	flPath = filepath.Join(flDir, "registry.json")

	// 1) disk se load (agar file mojood hai).
	diskOK := flLoadFromDisk()

	// 2) disk gayab → Storj se EK BAAR bulk-load (marker ke saath).
	marker := filepath.Join(flDir, "_loaded")
	if !diskOK {
		if _, err := os.Stat(marker); err != nil {
			InfoLog("FLEET-LOCAL: registry gayab — Storj se EK BAAR bulk-load...")
			n := flLoadFromStorjOnce()
			_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o644)
			InfoLog("FLEET-LOCAL: bulk-load done (%d sessions, %d servers)", n, len(flReg.Servers))
		} else {
			InfoLog("FLEET-LOCAL: registry gayab magar marker mojood — Storj load skip (bar bar nahi)")
		}
	}

	flMu.Lock()
	flReg.Self = fleetSelfID
	flReg.SelfURL = fleetSelfURL()
	flReg.Updated = time.Now().Unix()
	flMu.Unlock()
	flSave()

	flReady = true
	InfoLog("FLEET-LOCAL: ready (path=%s, sessions=%d, servers=%d)",
		flPath, len(flReg.Sessions), len(flReg.Servers))
}

// flStartRefresher: 60s MERGE-refresh (Storj se sirf badli hui entries).
// Local kabhi wipe nahi hota; apni fresh claim kabhi overwrite nahi hoti.
func flStartRefresher() {
	if !flEnabled || !flReady {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for range t.C {
			if !flReady || !storjReadyFlag() {
				continue
			}
			flMergeFromStorj()
		}
	}()
}

// ── disk I/O ────────────────────────────────────────────────────────────────

// flLoadFromDisk: registry.json padho. true = mil gaya.
func flLoadFromDisk() bool {
	data, err := os.ReadFile(flPath)
	if err != nil {
		return false
	}
	var reg flRegistry
	if json.Unmarshal(data, &reg) != nil {
		return false
	}
	if reg.Sessions == nil {
		reg.Sessions = map[string]*flSession{}
	}
	if reg.Servers == nil {
		reg.Servers = map[string]*flServer{}
	}
	flMu.Lock()
	flReg = &reg
	flMu.Unlock()
	return true
}

// flSave: atomic write (tmp + rename) — crash pe torn file nahi.
func flSave() {
	if !flEnabled || flPath == "" {
		return
	}
	flMu.Lock()
	flReg.Updated = time.Now().Unix()
	data, err := json.MarshalIndent(flReg, "", "  ")
	flMu.Unlock()
	if err != nil {
		return
	}
	tmp := flPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, flPath)
}

// ── Storj one-time load + merge ─────────────────────────────────────────────

// flLoadFromStorjOnce: Storj se servers hash + sessions set + per-JID claims
// padho aur local registry bharo. Sirf boot pe (disk gayab) call hota hai.
func flLoadFromStorjOnce() int {
	m := fleetMgr
	if m == nil || m.Redis == nil || !storjReadyFlag() {
		return 0
	}
	flMu.Lock()
	if flReg.Sessions == nil {
		flReg.Sessions = map[string]*flSession{}
	}
	if flReg.Servers == nil {
		flReg.Servers = map[string]*flServer{}
	}
	flMu.Unlock()

	// servers hash (heartbeat + URL + capacity)
	flMergeServersFromStorj()

	// sessions set + per-JID claims
	jids := m.Redis.setMembers(fleetSessionsSet)
	for _, jid := range jids {
		flMergeClaimFromStorj(jid)
	}
	flSave()
	return len(jids)
}

// flMergeFromStorj: continuous merge (60s). Sirf badli hui entries update.
func flMergeFromStorj() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	flMergeServersFromStorj()
	for _, jid := range m.Redis.setMembers(fleetSessionsSet) {
		flMergeClaimFromStorj(jid)
	}
	flSave()
}

// flMergeServersFromStorj: servers hash → local servers map (merge).
func flMergeServersFromStorj() {
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	r, err := m.Redis.cmd("HGETALL", fleetServersHash)
	if err != nil {
		return
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return
	}
	flMu.Lock()
	defer flMu.Unlock()
	for i := 0; i+1 < len(pairs); i += 2 {
		sid := pairs[i]
		val := strings.TrimSpace(pairs[i+1])
		ts, url, sess, mx := flParseHeartbeat(val)
		if ts <= 0 {
			continue
		}
		// 24h+ purani heartbeat → local se bhi hata do (hygiene).
		if time.Now().Unix()-ts > int64(fleetStaleServer/time.Second) {
			delete(flReg.Servers, sid)
			continue
		}
		cur := flReg.Servers[sid]
		if cur == nil {
			cur = &flServer{SID: sid}
			flReg.Servers[sid] = cur
		}
		// sirf fresher heartbeat se update (purani Storj value local ko
		// overwrite na kare).
		if ts >= cur.TS {
			cur.TS = ts
			if url != "" {
				cur.URL = url
			}
			cur.Sessions = sess
			cur.Max = mx
		}
	}
}

// flMergeClaimFromStorj: ek JID ka claim hash → local session entry (merge).
func flMergeClaimFromStorj(jid string) {
	m := fleetMgr
	if m == nil || m.Redis == nil || jid == "" {
		return
	}
	r, err := m.Redis.cmd("HGETALL", fleetClaimPrefix+jid)
	if err != nil {
		return
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return
	}
	flMu.Lock()
	defer flMu.Unlock()
	if flReg.Sessions == nil {
		flReg.Sessions = map[string]*flSession{}
	}
	cur := flReg.Sessions[jid]
	if cur == nil {
		cur = &flSession{JID: jid}
		flReg.Sessions[jid] = cur
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		sid := pairs[i]
		ts, _ := strconv.ParseInt(strings.TrimSpace(pairs[i+1]), 10, 64)
		if ts <= 0 {
			continue
		}
		// apni claim kabhi Storj se overwrite na ho (hum freshest hain).
		if sid == fleetSelfID {
			continue
		}
		// sirf fresher claim se update.
		if ts > cur.ClaimTS {
			cur.ServerID = sid
			cur.ClaimTS = ts
			cur.ServerURL = flServerURLFromLocal(sid)
			cur.Updated = time.Now().Unix()
		}
	}
}

// flParseHeartbeat: "<ts>|<sessions>|<max>|<url>" (purane "<ts>" bhi).
func flParseHeartbeat(val string) (ts int64, url string, sess, mx int) {
	sess, mx = -1, -1
	parts := strings.Split(val, "|")
	if len(parts) == 0 {
		return 0, "", -1, -1
	}
	ts, _ = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if len(parts) >= 2 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			sess = v
		}
	}
	if len(parts) >= 3 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
			mx = v
		}
	}
	if len(parts) >= 4 {
		u := strings.TrimSpace(parts[3])
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			url = strings.TrimSuffix(u, "/")
		}
	}
	return ts, url, sess, mx
}

// ── read API (local-first) ──────────────────────────────────────────────────

// flClaimHolders: local registry se {sid: ts}. (nil,false) = local me nahi.
func flClaimHolders(jid string) (map[string]int64, bool) {
	if !flEnabled || !flReady {
		return nil, false
	}
	flMu.Lock()
	defer flMu.Unlock()
	s := flReg.Sessions[jid]
	if s == nil || s.ServerID == "" || s.ClaimTS <= 0 {
		return nil, false
	}
	return map[string]int64{s.ServerID: s.ClaimTS}, true
}

// flHeartbeatMap: local registry se {sid: ts}.
func flHeartbeatMap() map[string]int64 {
	out := map[string]int64{}
	if !flEnabled || !flReady {
		return out
	}
	flMu.Lock()
	defer flMu.Unlock()
	for sid, srv := range flReg.Servers {
		if srv != nil && srv.TS > 0 {
			out[sid] = srv.TS
		}
	}
	return out
}

// flServerURLFromLocal: local registry se sid ka URL (lock held by caller).
func flServerURLFromLocal(sid string) string {
	if srv := flReg.Servers[sid]; srv != nil {
		return srv.URL
	}
	return ""
}

// flServerURL: local registry se sid ka URL (public API).
func flServerURL(sid string) string {
	if !flEnabled || !flReady {
		return ""
	}
	flMu.Lock()
	defer flMu.Unlock()
	return flServerURLFromLocal(sid)
}

// flSessionInfo: ek JID ki local info (copy).
func flSessionInfo(jid string) (flSession, bool) {
	if !flEnabled || !flReady {
		return flSession{}, false
	}
	flMu.Lock()
	defer flMu.Unlock()
	s := flReg.Sessions[jid]
	if s == nil {
		return flSession{}, false
	}
	return *s, true
}

// flAllSessions: saari local session entries (copy).
func flAllSessions() []flSession {
	if !flEnabled || !flReady {
		return nil
	}
	flMu.Lock()
	defer flMu.Unlock()
	out := make([]flSession, 0, len(flReg.Sessions))
	for _, s := range flReg.Sessions {
		if s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// ── write API (write-through) ───────────────────────────────────────────────

// flSetClaim: session ka claim holder local file me likho.
func flSetClaim(jid, sid, url string, ts int64) {
	if !flEnabled || !flReady || jid == "" {
		return
	}
	flMu.Lock()
	if flReg.Sessions == nil {
		flReg.Sessions = map[string]*flSession{}
	}
	s := flReg.Sessions[jid]
	if s == nil {
		s = &flSession{JID: jid}
		flReg.Sessions[jid] = s
	}
	if s.ServerID == sid && s.ClaimTS == ts {
		flMu.Unlock()
		return // no change — disk write skip (bandwidth/IO bachao)
	}
	s.ServerID = sid
	s.ServerURL = url
	s.ClaimTS = ts
	s.Updated = time.Now().Unix()
	flMu.Unlock()
	flSave()
}

// flSetOnline: session ka live status local file me likho.
func flSetOnline(jid string, online bool) {
	if !flEnabled || !flReady || jid == "" {
		return
	}
	flMu.Lock()
	if flReg.Sessions == nil {
		flReg.Sessions = map[string]*flSession{}
	}
	s := flReg.Sessions[jid]
	if s == nil {
		s = &flSession{JID: jid}
		flReg.Sessions[jid] = s
	}
	if s.Online == online && s.Updated > 0 {
		flMu.Unlock()
		return // no change — disk write skip
	}
	s.Online = online
	s.Updated = time.Now().Unix()
	flMu.Unlock()
	flSave()
}

// flSetServer: server heartbeat/URL local file me likho.
func flSetServer(sid, url string, ts int64, sess, mx int) {
	if !flEnabled || !flReady || sid == "" {
		return
	}
	flMu.Lock()
	if flReg.Servers == nil {
		flReg.Servers = map[string]*flServer{}
	}
	s := flReg.Servers[sid]
	if s == nil {
		s = &flServer{SID: sid}
		flReg.Servers[sid] = s
	}
	if s.TS == ts && (url == "" || s.URL == url) {
		flMu.Unlock()
		return // no change — disk write skip
	}
	s.TS = ts
	if url != "" {
		s.URL = url
	}
	s.Sessions = sess
	s.Max = mx
	flMu.Unlock()
	flSave()
}

// flRemove: session entry local file se hatao (logout/purge).
func flRemove(jid string) {
	if !flEnabled || !flReady || jid == "" {
		return
	}
	flMu.Lock()
	delete(flReg.Sessions, jid)
	flMu.Unlock()
	flSave()
}

// flRemoveClaim: sirf apna claim hatao (session entry rakho, online=false).
func flRemoveClaim(jid string) {
	if !flEnabled || !flReady || jid == "" {
		return
	}
	flMu.Lock()
	if s := flReg.Sessions[jid]; s != nil && s.ServerID == fleetSelfID {
		s.ServerID = ""
		s.ServerURL = ""
		s.ClaimTS = 0
		s.Updated = time.Now().Unix()
	}
	flMu.Unlock()
	flSave()
}

// ── OFFLINE NOTIFY (owner: "jese hi offline ho fleet walo ko info kr de") ────

// ── ONLINE GUARD (owner: "guard bithao jo folder ke sessions ko online check
//    karta rahe live har waqt; jese hi offline ho fleet ko info kr de") ───────

// flStartOnlineGuard: har 15s local folder ke sessions ka live status check.
//   • live    → local online=true (transition pe log).
//   • offline → agar pehle online tha (transition) → flNotifyOffline(jid):
//               local online=false + apna claim release + fail marker →
//               watchdog/resurrector us JID ko FREE server pe dispatch karega
//               aur wo EK BAAR claim karega.
// Sirf TRANSITION pe notify (har 15s spam nahi). 0 network (local check).
func flStartOnlineGuard(m *Manager) {
        if !flEnabled || m == nil {
                return
        }
        go func() {
                defer func() { _ = recover() }()
                t := time.NewTicker(15 * time.Second)
                defer t.Stop()
                for range t.C {
                        if m.IsShuttingDown() {
                                return
                        }
                        flOnlineGuardPass(m)
                }
        }()
}

// flOnlineGuardPass: ek pass — local folder ke har session ka status.
func flOnlineGuardPass(m *Manager) {
        jids := resurrectorDiskJIDs(m.cfg.PairingDir)
        for _, jid := range jids {
                if strings.HasPrefix(fleetUserPart(jid), "pending") || strings.HasPrefix(jid, "pending") {
                        continue
                }
                live := resurrectorLiveHere(m, jid)
                prev, had := flSessionInfo(jid)
                if live {
                        flSetOnline(jid, true)
                        continue
                }
                // offline: sirf transition pe notify (pehle online tha).
                if had && prev.Online {
                        flNotifyOffline(jid)
                } else if !had {
                        // pehli baar dekha + offline → entry banao (online=false).
                        flSetOnline(jid, false)
                }
        }
}

// ── KV MIRROR (central write-through hook) ──────────────────────────────────

// flMirrorKV: har fleet KV write ko local registry me mirror karo. cmdCore se
// call hota hai (dcApply ke baad). Isse claim/heartbeat/session-set ke SAARE
// write sites (fleet.go me 10+ jagah) automatically local file me reflect ho
// jate hain — koi call-site change nahi chahiye.
func flMirrorKV(args []string) {
        if !flEnabled || !flReady || len(args) < 2 {
                return
        }
        op := strings.ToUpper(args[0])
        key := args[1]

        switch {
        case strings.HasPrefix(key, fleetClaimPrefix):
                jid := strings.TrimPrefix(key, fleetClaimPrefix)
                if jid == "" {
                        return
                }
                switch op {
                case "HSET":
                        if len(args) >= 4 {
                                ts, _ := strconv.ParseInt(strings.TrimSpace(args[3]), 10, 64)
                                flSetClaim(jid, args[2], flServerURL(args[2]), ts)
                        }
                case "HDEL":
                        if len(args) >= 3 && args[2] == fleetSelfID {
                                flRemoveClaim(jid)
                        }
                case "DEL":
                        flRemove(jid)
                }
        case key == fleetServersHash:
                if op == "HSET" && len(args) >= 4 {
                        ts, url, sess, mx := flParseHeartbeat(args[3])
                        if ts > 0 {
                                flSetServer(args[2], url, ts, sess, mx)
                        }
                }
        case key == fleetSessionsSet:
                switch op {
                case "SADD":
                        for _, jid := range args[2:] {
                                flSetOnline(jid, false) // entry banao (status baad me update)
                        }
                case "SREM":
                        for _, jid := range args[2:] {
                                flRemove(jid)
                        }
                }
        }
}

// flNotifyOffline: session offline ho gaya → local online=false + apna claim
// release (Storj HDEL + local) + fail-marker (doosre server ko pata chale).
// Iske baad watchdog/resurrector us JID ko FREE server pe dispatch karta hai
// aur wo server EK BAAR claim karta hai.
func flNotifyOffline(jid string) {
	if jid == "" {
		return
	}
	flSetOnline(jid, false)
	flRemoveClaim(jid)
	m := fleetMgr
	if m == nil || m.Redis == nil {
		return
	}
	// apna claim Storj se bhi hatao (doosre server ko free dikhe).
	_, _ = m.Redis.cmd("HDEL", fleetClaimPrefix+jid, fleetSelfID)
	// fail-marker: "ye session abhi kisi ke paas nahi, claim karo".
	_ = m.Redis.setString(fleetFailMarkPrefix+jid, fleetSelfID)
	JSONDebug("FLEET_LOCAL_OFFLINE_NOTIFY", map[string]any{
		"jid": jid, "self": fleetSelfID, "action": "claim-released-notify",
	})
	InfoLog("FLEET-LOCAL: %s OFFLINE — claim release + fleet notify (doosra server claim karega)", jid)
}
