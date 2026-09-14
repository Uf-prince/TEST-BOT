package main

// ═══════════════════════════════════════════════════════════════════════════════
//   SELF-PING KEEP-ALIVE (Render free tier anti-sleep) — owner order:
//   "Render 15 min me service sleep kr deta hai — har naye deploy pe
//   UptimeRobot me ja ke URL lagwana parta hai. Aisa system banao ke bot
//   deploy hote hi khud apne server ko har 5 min me ping karta rahe —
//   speed 0% farak, RAM 0% asar, bot hamesha chalta rahe."
//
//   KAAM KAISE KARTA HAI:
//   - Render har deploy pe RENDER_EXTERNAL_URL env var automatic set karta
//     hai (e.g. https://gold-md-xsvr1.onrender.com).
//   - Ye module us URL (ya SELF_PING_URL env override) ke /health pe
//     har 5 min me ek chhota GET mar-ta hai — in-bound traffic = service
//     kabhi idle/sleep nahi hota.
//   - Local/panel deploys pe RENDER_EXTERNAL_URL set NAHI hota →
//     keep-alive silently inactive rehta hai (zero network calls).
//   - Ping ek goroutine me ticker se chalta hai (RAM footprint ~0 —
//     sirf 1 goroutine + 1 http.Client reuse). Bot ki speed/throughput
//     par ZERO asar (commands/background jobs isi tarah chalte hain).
//   - Off karna ho: env KEEPALIVE_ENABLED=false.
// ════════════════════════════════════════════════════════════════════════════════

import (
	"net/http"
	"os"
	"time"
)

// keepaliveClient: sirf ek reusable client (zero extra alloc per ping).
// Timeout 10s — Render cold-boot bhi handle ho jaye (sleep me ping bhi
// uthega hi, bas thoda der lage).
var keepaliveClient = &http.Client{Timeout: 10 * time.Second}

// keepaliveTargetURL: ping karne wala public URL (RENDER_EXTERNAL_URL ya
// SELF_PING_URL env). Koi URL nahi → keep-alive inactive.
func keepaliveTargetURL() string {
	u := os.Getenv("RENDER_EXTERNAL_URL")
	if u == "" {
		u = os.Getenv("SELF_PING_URL")
	}
	if u == "false" || u == "0" || u == "off" {
		return ""
	}
	// trailing slash hatao, https:// force nahi — Render URL wahi use karo
	return trimTrailingSlash(u)
}

// trimTrailingSlash: "https://x.onrender.com/" → "https://x.onrender.com"
func trimTrailingSlash(u string) string {
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	// "https://" scheme ke andar wale slashes ko chhoda
	if u == "https:" || u == "http:" {
		return ""
	}
	return u
}

// keepaliveIntervalMinutes: ping interval (default 5 min, env override).
// Render free tier: 15 min idle pe sleep → 5 min safe margin.
func keepaliveIntervalMinutes() int {
	n := envInt("SELF_PING_MINUTES", 5)
	if n < 1 {
		n = 1
	}
	if n > 60 {
		n = 60
	}
	// 15-min sleep window se kam rakhna zaroori — default 5
	return n
}

// selfPingOnce: ek ping. Fail (network glitch) → sirf next tick retry,
// koi error path bot ko touch nahi karta.
func selfPingOnce(target string) bool {
	req, err := http.NewRequest("GET", target+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "GOLDMD-KEEPALIVE/1.0")
	resp, err := keepaliveClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// status jo bhi ho — 200 ya 503 — traffic aa gayi, sleep reset ho gaya.
	// body read drain (connection reuse ke liye) — 64KB tak:
	buf := make([]byte, 4096)
	for i := 0; i < 16; i++ { // 16*4KB = 64KB max drain
		if n, err := resp.Body.Read(buf); err != nil || n == 0 {
			break
		}
	}
	return true
}

// StartSelfPingKeepAlive: main() se call hota hai (panel start ke baad,
// goroutine me — pehla ping 30s delay se, bot boot pe 0% asar).
// Koi URL nahi → return (inactive). Ticker goroutine 5-min ping loop.
func StartSelfPingKeepAlive() {
	if os.Getenv("KEEPALIVE_ENABLED") == "false" {
		return
	}
	target := keepaliveTargetURL()
	if target == "" {
		// No public URL (local/panel runs) → keep-alive inactive.
		return
	}
	interval := time.Duration(keepaliveIntervalMinutes()) * time.Minute
	go func() {
		// Startup ke 30s baad pehla ping — panel up hone do.
		time.Sleep(30 * time.Second)
		selfPingOnce(target)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			selfPingOnce(target)
		}
	}()
	InfoLog("SELF-PING KEEP-ALIVE ON → %s/health har %s me (Render anti-sleep)",
		target, interval)
}

// ═══════════════════════════════════════════════════════════════════════════════
//   NOTE (deploy checklist — Render pe kya hoga):
//   1. Render deploy → RENDER_EXTERNAL_URL automatic set ho jata hai.
//   2. Bot start → keep-alive 30s baad pehla ping, phir har 5 min.
//   3. UptimeRobot me URL lagane ki zaroorat KHATAM — bot khud ping karega.
//   4. Same code local pe chalega (URL env nahi → inactive, koi side effect
//      nahi — local bot me keep-alive kabhi ping nahi karega).
//   5. Multi-server: har Render service apna RENDER_EXTERNAL_URL rakhti
//      hai → har deploy pe apna keep-alive automatic ON.
// ═══════════════════════════════════════════════════════════════════════════════
