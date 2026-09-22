#!/usr/bin/env python3
# RECONNECT-WAR FIX (owner: jani — "reconnecter fleet wale problems set kr k fix kro")
#
# ROOT CAUSE (sandbox fleet invisible): svr11221 ka koi public URL fleet me
#   publish nahi hota tha → fleetServerURL("svr11221") = "" → holder-alive
#   probe fail + guardRemoteServers me sandbox missing → online-elsewhere
#   detection kabhi sahi nahi chala → double-connect = stream-replace war.
#
# PART A: /health me per-session array ("sess":[{"jid","online"}]).
# PART B: heartbeat me apna public URL publish ("<ts>|<sessions>|<max>|<url>")
#         + fleetServerURL heartbeat-fallback + guardRemoteServers me
#         heartbeat URLs merge (servers.json + tunnel dono).
# PART C: handleFleetDispatch/resurrectorPass online-first gate READ-VERIFIED
#         (fleetSessionOnlineElsewhere STEP1 claim + STEP2 probe wave +
#         dispatch me double-check) — code already correct tha, B ke baad
#         ab ye sandbox ko bhi dekhenge.
import re

OK = []

def patch(path, subs):
    src = open(path, encoding='utf-8').read()
    for old, new, tag in subs:
        assert old in src, f"{path}: anchor MISSING for {tag}"
        assert src.count(old) == 1, f"{path}: anchor not unique for {tag}"
        src = src.replace(old, new)
    open(path, 'w', encoding='utf-8').write(src)
    OK.append(path)

# ══════════ fleet.go ══════════
patch('src/fleet.go', [

# B2 — fleetSelfURL helper + heartbeat 4th segment
('''	// NAYA format: "<ts>|<sessions>|<max>" — Session Resurrector isse pata
	// lagata hai kaunsa server FREE hai (sessions < max) bina kisi HTTP
	// probe ke. Purana "<ts>" format bhi parse hota rehta hai (niche).
	val := strconv.FormatInt(time.Now().Unix(), 10) + "|" +
		strconv.Itoa(fleetMgr.SlotsUsed()) + "|" +
		strconv.Itoa(maxPairedSessions())
	_, _ = fleetMgr.Redis.cmd("HSET", fleetServersHash, fleetSelfID, val)''',
'''	// NAYA format: "<ts>|<sessions>|<max>|<url>" — Session Resurrector isse
	// pata lagata hai kaunsa server FREE hai (sessions < max) bina kisi HTTP
	// probe ke, AUR (owner fix B) 4th segment = is server ka public URL —
	// hostname-style sid (svr11221) ka URL ab fleet me discoverable hai.
	// Purana "<ts>" / "<ts>|<sessions>|<max>" format bhi parse hota rehta hai.
	val := strconv.FormatInt(time.Now().Unix(), 10) + "|" +
		strconv.Itoa(fleetMgr.SlotsUsed()) + "|" +
		strconv.Itoa(maxPairedSessions()) + "|" +
		fleetSelfURL()
	_, _ = fleetMgr.Redis.cmd("HSET", fleetServersHash, fleetSelfID, val)''', 'B2-heartbeat'),

# B2b — fleetSelfURL function (fleetHeartbeat ke baad daalo)
('''// fleetOrphanSweep releases claims held by dead servers''',
'''// fleetSelfURL: is server ka PUBLIC URL (fleet discoverability — owner fix
// B, 2026-09-16). GOLDMD_PUBLIC_URL (launch_fresh.sh me tunnel URL set hota
// hai) → RENDER_EXTERNAL_URL → "". Heartbeat ke 4th segment me jata hai;
// fleetServerURL isko hostname-style sid ke liye fallback me use karta hai.
func fleetSelfURL() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_PUBLIC_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return ""
}

// fleetURLFromHeartbeat: ek sid ka public URL heartbeat hash se (4th segment).
// fleetServerURL ka fallback — hostname-style sid (dot nahi) pe pehle ""
//// tha, ab heartbeat URL milta hai (cmd cache 3min TTL — sasta).
func fleetURLFromHeartbeat(sid string) string {
	if sid == "" || fleetMgr == nil || fleetMgr.Redis == nil {
		return ""
	}
	raw, err := fleetMgr.Redis.cmd("HGET", fleetServersHash, sid)
	if err != nil || len(raw) == 0 {
		return ""
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	parts := strings.Split(v, "|")
	if len(parts) < 4 {
		return ""
	}
	u := strings.TrimSpace(parts[3])
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return strings.TrimSuffix(u, "/")
	}
	return ""
}

// fleetHeartbeatURLs: servers hash → {sid: url} (ek hi HGETALL, 4th segment).
// guardRemoteServers isse sandbox/tunnel servers ko probe list me include
// karta hai (pehle sirf servers.json tha — sandbox invisible tha).
func fleetHeartbeatURLs() map[string]string {
	out := map[string]string{}
	if fleetMgr == nil || fleetMgr.Redis == nil {
		return out
	}
	r, err := fleetMgr.Redis.cmd("HGETALL", fleetServersHash)
	if err != nil {
		return out
	}
	var pairs []string
	if json.Unmarshal(r, &pairs) != nil {
		return out
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		parts := strings.Split(strings.TrimSpace(pairs[i+1]), "|")
		if len(parts) < 4 {
			continue
		}
		u := strings.TrimSpace(parts[3])
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			out[pairs[i]] = strings.TrimSuffix(u, "/")
		}
	}
	return out
}

// fleetOrphanSweep releases claims held by dead servers''', 'B2b-helpers'),

# B3 — fleetServerURL heartbeat fallback
('''	// hostname-style sid → Render external URL pattern nahi banta —
	// heuristic: pure-hostname ya onrender.com ho to https:// prefix.
	if strings.Contains(sid, ".") {
		return "https://" + strings.TrimSuffix(sid, "/")
	}
	return ""
}''',
'''	// hostname-style sid → Render external URL pattern nahi banta —
	// heuristic: pure-hostname ya onrender.com ho to https:// prefix.
	if strings.Contains(sid, ".") {
		return "https://" + strings.TrimSuffix(sid, "/")
	}
	// OWNER FIX B (2026-09-16): hostname-style sid (svr11221 jaisa) pehle
	// "" return karta tha → holder-alive probe dead → claims orphan-sweep
	// → double-connect war. Ab heartbeat me published URL se resolve karo.
	return fleetURLFromHeartbeat(sid)
}''', 'B3-serverURL'),

# A — fleetWriteHealth sess array
('''func fleetWriteHealth(w io.Writer, sessions int) {
	fmt.Fprintf(w, `{"bot":"GOLD-MD","status":"online","sessions":%d,"max":%d,"re":"%s","sid":"%s","used_mb":%.1f}`,
		sessions, maxPairedSessions(), fleetREStatus(), fleetSelfID, fleetEgressUsedMB())
}''',
'''func fleetWriteHealth(w io.Writer, sessions int) {
	// OWNER FIX A (2026-09-16): per-session online array — remote servers
	// ek hi /health request me liveness + session-state dono dekh lete
	// hain (purane readers unknown field ignore karte hain — safe).
	sessJSON := "[]"
	if m := fleetMgr; m != nil {
		type row struct {
			JID    string `json:"jid"`
			Online bool   `json:"online"`
		}
		rows := make([]row, 0, 8)
		for _, s := range m.List() {
			online := s.Paired && s.Client != nil && s.Client.IsConnected() &&
				s.Client.Store != nil && s.Client.Store.ID != nil
			rows = append(rows, row{JID: s.JID, Online: online})
		}
		if b, err := json.Marshal(rows); err == nil {
			sessJSON = string(b)
		}
	}
	fmt.Fprintf(w, `{"bot":"GOLD-MD","status":"online","sessions":%d,"max":%d,"re":"%s","sid":"%s","used_mb":%.1f,"sess":%s}`,
		sessions, maxPairedSessions(), fleetREStatus(), fleetSelfID, fleetEgressUsedMB(), sessJSON)
}''', 'A-health-sess'),
])

# ══════════ fleet.go: fleetHealthFields Sess field ══════════
patch('src/fleet.go', [
('''type fleetHealthFields struct {
	Bot      string  `json:"bot"`
	Status   string  `json:"status"`
	Sessions int     `json:"sessions"`
	Max      int     `json:"max"`
	RE       string  `json:"re"`
	SID      string  `json:"sid"`
	UsedMB   float64 `json:"used_mb"`
}''',
'''type fleetHealthFields struct {
	Bot      string  `json:"bot"`
	Status   string  `json:"status"`
	Sessions int     `json:"sessions"`
	Max      int     `json:"max"`
	RE       string  `json:"re"`
	SID      string  `json:"sid"`
	UsedMB   float64 `json:"used_mb"`
	Sess     []struct {
		JID    string `json:"jid"`
		Online bool   `json:"online"`
	} `json:"sess"`
}''', 'A-healthFields'),
])

# ══════════ session_guards.go: guardRemoteServers heartbeat URLs merge ══════════
patch('src/session_guards.go', [
('''func guardRemoteServers() []string {
	if !fleetActive() {
		return nil
	}
	loadServersConfig()
	cfg := serversCfg
	out := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if s.URL == "" {
			continue
		}
		out = append(out, strings.TrimRight(s.URL, "/"))
	}
	return out
}''',
'''func guardRemoteServers() []string {
	if !fleetActive() {
		return nil
	}
	// OWNER FIX B (2026-09-16): servers.json ke URLs + heartbeat me published
	// URLs — sandbox/tunnel (svr11221) servers.json me nahi tha is liye
	// online-elsewhere probe wave usko MISS karta tha → double-connect war.
	// Ab heartbeat URL wale servers bhi probe list me hain.
	seen := map[string]bool{}
	out := make([]string, 0, len(serversCfg.Servers)+4)
	add := func(u string) {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		out = append(out, u)
	}
	loadServersConfig()
	for _, s := range serversCfg.Servers {
		add(s.URL)
	}
	for sid, u := range fleetHeartbeatURLs() {
		if sid == fleetSelfID {
			continue // apna URL nahi — hum khud probe nahi karte
		}
		add(u)
	}
	return out
}''', 'B4-guardURLs'),
])

print("ALL RECONNECT-WAR PATCHES OK:", OK)
