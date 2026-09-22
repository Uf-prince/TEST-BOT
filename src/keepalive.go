package main

// ═══════════════════════════════════════════════════════════════════════════════
//   SELF-PING KEEP-ALIVE (Render free tier anti-sleep) — owner order:
//   "Render 15 min me service sleep kr deta hai. Aisa system banao ke bot
//    deploy hote hi khud apne server ko HAR 10 MIN me ping karta rahe —
//    speed 0% farak, RAM 0% asar, bot hamesha chalta rahe."
//
//   KAAM KAISE KARTA HAI:
//   - Render har deploy pe RENDER_EXTERNAL_URL env var automatic set karta
//     hai (e.g. https://gold-md-xsvrr36.onrender.com). Us URL (ya SELF_PING_URL
//     / GOLDMD_SELF_URL / RENDER_EXTERNAL_HOSTNAME / servers.json fallback) ke
//     /health pe har 10 min me ek chhota GET — in-bound traffic = service kabhi
//     idle/sleep nahi hota (Render ka idle window 15 min hai, 10 min = safe).
//   - Local/panel deploys pe koi URL set NAHI hota → keep-alive silently
//     inactive rehta hai (zero network calls, zero goroutine).
//   - Ping ek goroutine me ticker se chalta hai (RAM footprint ~0 — sirf
//     1 goroutine + 1 reusable http.Client). Bot ki speed par ZERO asar.
//   - Off karna ho: env KEEPALIVE_ENABLED=off (ya false / 0).
//
//   VERIFY KAISE KAREIN (bahar se, bina Render dashboard):
//     curl https://<server>.onrender.com/health
//     → JSON me "keepalive":{"every_min":10,"pings":N,"last_ok":true,...}
//     "pings" har 10 min me badhta hai = anti-sleep WAQAI chal raha hai.
// ═══════════════════════════════════════════════════════════════════════════════

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// keepaliveClient: sirf ek reusable client (zero extra alloc per ping).
// Timeout 20s — Render cold-boot (sleep se uthna) bhi handle ho jaye.
var keepaliveClient = &http.Client{Timeout: 20 * time.Second}

// keepaliveState: public /health ke liye live proof (kitne ping, kab, kya
// result). Sirf counters — 0 extra network cost.
type keepaliveState struct {
	mu        sync.Mutex
	target    string
	everyMin  int
	pings     int64
	okCount   int64
	failCount int64
	lastTS    int64
	lastOK    bool
	lastErr   string
}

var keepaliveInfo = &keepaliveState{}

// keepaliveEnabled: master switch (KEEPALIVE_ENABLED=off/false/0 disables).
func keepaliveEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KEEPALIVE_ENABLED"))) {
	case "off", "false", "0", "no", "disabled":
		return false
	}
	return true
}

// keepaliveFirstNonEmpty: pehla non-empty trimmed value.
func keepaliveFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// keepaliveURLForServerName: servers.json me GOLDMD_SERVER_NAME (e.g.
// "SERVER 42") ka URL dhoondo — last-resort fallback jab Render apna URL
// kisi env me na de (self-hosted / custom host).
func keepaliveURLForServerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	loadServersConfig()
	for _, s := range serversCfg.Servers {
		if strings.EqualFold(strings.TrimSpace(s.Name), name) {
			return strings.TrimSpace(s.URL)
		}
	}
	return ""
}

// keepaliveTargetURL: ping karne wala public URL. Priority:
//
//	RENDER_EXTERNAL_URL → SELF_PING_URL → GOLDMD_SELF_URL →
//	https://RENDER_EXTERNAL_HOSTNAME → servers.json[GOLDMD_SERVER_NAME]
//
// Koi URL nahi → keep-alive inactive ("" return).
func keepaliveTargetURL() string {
	u := keepaliveFirstNonEmpty(
		os.Getenv("RENDER_EXTERNAL_URL"),
		os.Getenv("SELF_PING_URL"),
		os.Getenv("GOLDMD_SELF_URL"),
	)
	if u == "" {
		if host := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_HOSTNAME")); host != "" {
			u = "https://" + host
		}
	}
	if u == "" {
		u = keepaliveURLForServerName(os.Getenv("GOLDMD_SERVER_NAME"))
	}
	if u == "" {
		return ""
	}
	// explicit kill-values
	switch strings.ToLower(u) {
	case "false", "0", "off", "no", "none":
		return ""
	}
	// scheme missing ho to https lagao (hostname-only value ke liye)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "https://" + u
	}
	return trimTrailingSlash(u)
}

// trimTrailingSlash: "https://x.onrender.com/" → "https://x.onrender.com"
func trimTrailingSlash(u string) string {
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	if u == "https:" || u == "http:" {
		return ""
	}
	return u
}

// keepaliveIntervalMinutes: ping interval (default 10 min, env override).
// Render free tier: 15 min idle pe sleep → 10 min safe margin (2/3 of window,
// ek ping miss ho jaye to bhi service sleep nahi hoti).
func keepaliveIntervalMinutes() int {
	n := envInt("SELF_PING_MINUTES", 10)
	if n < 1 {
		n = 1
	}
	if n > 60 {
		n = 60
	}
	return n
}

// selfPingOnce: ek ping. Fail (network glitch) → sirf next tick retry,
// koi error path bot ko touch nahi karta. Result state me record hota hai
// (/health se verify karne ke liye).
func selfPingOnce(target string) bool {
	ok, errText := selfPingDo(target)
	keepaliveInfo.mu.Lock()
	keepaliveInfo.pings++
	keepaliveInfo.lastTS = time.Now().Unix()
	keepaliveInfo.lastOK = ok
	keepaliveInfo.lastErr = errText
	if ok {
		keepaliveInfo.okCount++
	} else {
		keepaliveInfo.failCount++
	}
	keepaliveInfo.mu.Unlock()
	return ok
}

// selfPingDo: actual GET {target}/health (stats se alag — testable).
func selfPingDo(target string) (bool, string) {
	req, err := http.NewRequest("GET", target+"/health", nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("User-Agent", "GOLDMD-KEEPALIVE/1.0")
	resp, err := keepaliveClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	// status jo bhi ho (200/503) — in-bound traffic aa gayi, idle timer reset.
	// body drain (connection reuse ke liye) — 64KB tak.
	buf := make([]byte, 4096)
	for i := 0; i < 16; i++ {
		if n, err := resp.Body.Read(buf); err != nil || n == 0 {
			break
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, ""
	}
	return true, "" // traffic delivered — Render ke liye ye hi maayne rakhta hai
}

// keepaliveHealthJSON: /health ke JSON me append hone wala fragment.
func keepaliveHealthJSON() string {
	keepaliveInfo.mu.Lock()
	defer keepaliveInfo.mu.Unlock()
	active := "false"
	if keepaliveInfo.target != "" {
		active = "true"
	}
	return "{\"active\":" + active +
		",\"every_min\":" + kaItoa(keepaliveInfo.everyMin) +
		",\"pings\":" + kaItoa64(keepaliveInfo.pings) +
		",\"ok\":" + kaItoa64(keepaliveInfo.okCount) +
		",\"fail\":" + kaItoa64(keepaliveInfo.failCount) +
		",\"last_ts\":" + kaItoa64(keepaliveInfo.lastTS) +
		",\"last_ok\":" + kaBoolStr(keepaliveInfo.lastOK) +
		",\"target\":\"" + keepaliveInfo.target + "\"}"
}

// StartSelfPingKeepAlive: main() se call hota hai (panel start ke baad,
// goroutine me). Koi URL nahi → return (inactive). Ticker goroutine
// har {SELF_PING_MINUTES, default 10} min me apne /health ko ping karti hai.
func StartSelfPingKeepAlive() {
	if !keepaliveEnabled() {
		InfoLog("SELF-PING KEEP-ALIVE OFF (KEEPALIVE_ENABLED)")
		return
	}
	target := keepaliveTargetURL()
	if target == "" {
		// No public URL (local/panel runs) → keep-alive inactive.
		InfoLog("SELF-PING KEEP-ALIVE idle — koi public URL env nahi (RENDER_EXTERNAL_URL / SELF_PING_URL / GOLDMD_SELF_URL)")
		return
	}
	minutes := keepaliveIntervalMinutes()
	interval := time.Duration(minutes) * time.Minute

	keepaliveInfo.mu.Lock()
	keepaliveInfo.target = target
	keepaliveInfo.everyMin = minutes
	keepaliveInfo.mu.Unlock()

	go func() {
		defer func() { _ = recover() }()
		// Startup ke 30s baad pehla ping — panel up hone do.
		time.Sleep(30 * time.Second)
		selfPingOnce(target)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			selfPingOnce(target)
		}
	}()
	InfoLog("SELF-PING KEEP-ALIVE ON → %s/health har %d min me (Render anti-sleep)",
		target, minutes)
}

// itoa / kaItoa64 / boolStr: chhote helpers (fmt import bachane ke liye).
func kaItoa(n int) string {
	return kaItoa64(int64(n))
}

func kaItoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func kaBoolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// ═══════════════════════════════════════════════════════════════════════════════
//   DEPLOY CHECKLIST (Render):
//   1. Render deploy → RENDER_EXTERNAL_URL automatic set ho jata hai.
//   2. Bot start → 30s baad pehla ping, phir HAR 10 MIN (Render sleep = 15 min).
//   3. UptimeRobot / cron-job.org ki zaroorat KHATAM — bot khud ping karta hai.
//   4. Verify: curl <url>/health → "keepalive":{"active":true,"every_min":10,
//      "pings":N,...} — N har 10 min me barhta hai.
//   5. Same code local pe: URL env nahi → inactive, koi side effect nahi.
//   6. Multi-server: har Render service ka apna RENDER_EXTERNAL_URL → apna
//      keep-alive. Ek server dusre ko ping nahi karta.
// ═══════════════════════════════════════════════════════════════════════════════
