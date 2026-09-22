package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// KEEP-ALIVE (anti-sleep) — REAL end-to-end proof:
// ek local HTTP server /health serve karta hai, phir selfPingOnce usi ko hit
// karta hai. Ye test WAQAI network round-trip karta hai (mock nahi).
// ─────────────────────────────────────────────────────────────────────────────

func TestKeepalivePingsOwnHealthEndpoint(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			atomic.AddInt64(&hits, 1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"bot":"GOLD-MD","status":"online"}`))
	}))
	defer srv.Close()

	t.Setenv("KEEPALIVE_ENABLED", "") // default = ON
	t.Setenv("SELF_PING_URL", srv.URL)

	if !keepaliveEnabled() {
		t.Fatalf("keepalive must default to ON")
	}

	target := keepaliveTargetURL()
	if target != srv.URL {
		t.Fatalf("keepaliveTargetURL() = %q, want %q", target, srv.URL)
	}

	// StartSelfPingKeepAlive ke bina state me target set karo (unit-level).
	keepaliveInfo.mu.Lock()
	keepaliveInfo.target = srv.URL
	keepaliveInfo.everyMin = 10
	keepaliveInfo.mu.Unlock()

	before := atomic.LoadInt64(&hits)
	if !selfPingOnce(target) {
		t.Fatalf("selfPingOnce returned false — ping failed")
	}
	after := atomic.LoadInt64(&hits)
	if after-before != 1 {
		t.Fatalf("health endpoint hits = %d, want exactly 1 new hit", after-before)
	}

	js := keepaliveHealthJSON()
	for _, want := range []string{`"pings":`, `"every_min":`, `"last_ok":true`, `"target":"` + srv.URL + `"`} {
		if !strings.Contains(js, want) {
			t.Errorf("/health keepalive fragment missing %q in: %s", want, js)
		}
	}
	if !strings.Contains(js, `"pings":1`) {
		t.Errorf("ping counter not recorded: %s", js)
	}
}

// Interval default = 10 min (owner order: Render 15-min sleep se pehle ping).
func TestKeepaliveIntervalDefaultsTo10Minutes(t *testing.T) {
	t.Setenv("SELF_PING_MINUTES", "")
	if got := keepaliveIntervalMinutes(); got != 10 {
		t.Fatalf("default interval = %d min, want 10", got)
	}
	t.Setenv("SELF_PING_MINUTES", "7")
	if got := keepaliveIntervalMinutes(); got != 7 {
		t.Fatalf("env override = %d min, want 7", got)
	}
	t.Setenv("SELF_PING_MINUTES", "999")
	if got := keepaliveIntervalMinutes(); got != 60 {
		t.Fatalf("clamp = %d min, want 60", got)
	}
}

// Target resolution chain: RENDER_EXTERNAL_URL > SELF_PING_URL > GOLDMD_SELF_URL
// > RENDER_EXTERNAL_HOSTNAME. Koi env nahi → inactive (empty).
func TestKeepaliveTargetResolution(t *testing.T) {
	t.Setenv("RENDER_EXTERNAL_URL", "")
	t.Setenv("SELF_PING_URL", "")
	t.Setenv("GOLDMD_SELF_URL", "")
	t.Setenv("RENDER_EXTERNAL_HOSTNAME", "")
	t.Setenv("GOLDMD_SERVER_NAME", "")
	if got := keepaliveTargetURL(); got != "" {
		t.Fatalf("no env → want inactive (empty), got %q", got)
	}

	t.Setenv("RENDER_EXTERNAL_URL", "https://gold-md-xsvrr36.onrender.com/")
	if got := keepaliveTargetURL(); got != "https://gold-md-xsvrr36.onrender.com" {
		t.Fatalf("trailing slash not trimmed: %q", got)
	}

	t.Setenv("RENDER_EXTERNAL_URL", "")
	t.Setenv("RENDER_EXTERNAL_HOSTNAME", "gold-md-xsvrr36.onrender.com")
	if got := keepaliveTargetURL(); got != "https://gold-md-xsvrr36.onrender.com" {
		t.Fatalf("hostname fallback failed: %q", got)
	}

	// Kill switch.
	t.Setenv("KEEPALIVE_ENABLED", "off")
	if keepaliveEnabled() {
		t.Fatalf("KEEPALIVE_ENABLED=off must disable")
	}
}

// REAL HTTP round-trip: asli Manager.HealthHandler ko httptest server pe chalao
// aur response body me keepalive block verify karo (ye wahi handler hai jo
// Render pe /health serve karta hai).
func TestHealthEndpointServesKeepaliveOverHTTP(t *testing.T) {
	mgr := &Manager{}
	srv := httptest.NewServer(http.HandlerFunc(mgr.HealthHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", resp.StatusCode, out)
	}
	for _, want := range []string{`"bot":"GOLD-MD"`, `"sessions":0`, `"keepalive":{`, `"every_min":`} {
		if !strings.Contains(out, want) {
			t.Errorf("/health body missing %q in: %s", want, out)
		}
	}
}

// /health payload me keepalive block hona chahiye (deploy verify isi se hota hai).
func TestFleetHealthJSONCarriesKeepaliveBlock(t *testing.T) {
	var buf bytes.Buffer
	fleetWriteHealth(&buf, 3)
	out := buf.String()
	for _, want := range []string{`"sessions":3`, `"keepalive":{`, `"active":`, `"every_min":`} {
		if !strings.Contains(out, want) {
			t.Errorf("/health JSON missing %q in: %s", want, out)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// FLEET FAILOVER — decision logic (dead peer = failover, live peer = NO steal)
// ─────────────────────────────────────────────────────────────────────────────

// Dead-server failover ab DEFAULT ON hai (pehle false tha → sessions kabhi
// dusre server pe online nahi hote the).
func TestFleetHandoffEnabledDefaultsOn(t *testing.T) {
	t.Setenv("GOLDMD_FLEET_HANDOFF", "")
	if !fleetHandoffEnabled() {
		t.Fatalf("fleet handoff must default to ON (dead-server failover)")
	}
	t.Setenv("GOLDMD_FLEET_HANDOFF", "off")
	if fleetHandoffEnabled() {
		t.Fatalf("GOLDMD_FLEET_HANDOFF=off must disable handoff")
	}
}

func TestFleetProbeAfterTightened(t *testing.T) {
	if fleetProbeAfter > 3*time.Minute {
		t.Fatalf("fleetProbeAfter = %v, want <= 3m (failover must be fast)", fleetProbeAfter)
	}
	if fleetProbeAfter < time.Minute {
		t.Fatalf("fleetProbeAfter = %v, too aggressive (false-DEAD risk)", fleetProbeAfter)
	}
}

// LVENESS MATRIX: fresh heartbeat → ALIVE (claim nahi chheena jayega);
// stale heartbeat + unreachable peer → DEAD (failover chalega).
func TestFleetHolderLivenessDecisions(t *testing.T) {
	savedSelf := fleetSelfID
	defer func() { fleetSelfID = savedSelf }()
	fleetSelfID = "gold-md-self.onrender.com"

	// 1) FRESH heartbeat (30s) → ALIVE, no probe → NO steal (war-safe).
	hb := map[string]int64{"gold-md-live.onrender.com": time.Now().Unix() - 30}
	if !fleetHolderAlive("gold-md-live.onrender.com", hb) {
		t.Errorf("fresh heartbeat holder must be ALIVE (war-safe, no steal)")
	}

	// 2) STALE heartbeat (4 min) + unreachable peer → DEAD → failover allowed.
	dead := map[string]int64{"127.0.0.1:1": time.Now().Unix() - 240}
	if fleetHolderAlive("127.0.0.1:1", dead) {
		t.Errorf("stale heartbeat + unreachable peer must be DEAD (failover must fire)")
	}

	// 3) Heartbeat hi nahi → DEAD.
	if fleetHolderAlive("gold-md-gone.onrender.com", map[string]int64{}) {
		t.Errorf("missing heartbeat must be DEAD")
	}

	// 4) Self always alive.
	if !fleetHolderAlive(fleetSelfID, map[string]int64{}) {
		t.Errorf("self must always be ALIVE")
	}
}

// Fresh claim (<10min) = trust (koi probe nahi) — double-connect war se bachne
// ke liye. Ye constant hi failover ki safety window hai.
func TestFleetClaimFreshTrustWindow(t *testing.T) {
	if fleetClaimFreshTrust < 5*time.Minute || fleetClaimFreshTrust > 15*time.Minute {
		t.Fatalf("fleetClaimFreshTrust = %v, want 5m..15m (race-safe window)", fleetClaimFreshTrust)
	}
}

// .server FRESH scan: cache clear karne ke baad cache khali hona chahiye
// (fake/stale data kabhi na dikhe).
func TestFleetScanAllFreshClearsHealthCache(t *testing.T) {
	fleetHealthCacheMu.Lock()
	fleetHealthCache = map[string]fleetHealthCacheEntry{
		"https://gold-md-fake.onrender.com": {hf: nil, ts: time.Now()},
	}
	fleetHealthCacheMu.Unlock()

	// servers.json khali/absent ho to bhi panic nahi hona chahiye.
	_ = fleetScanAllFresh()

	fleetHealthCacheMu.Lock()
	n := len(fleetHealthCache)
	fleetHealthCacheMu.Unlock()
	// cache fresh scan ke dauraan naye results se bhar sakta hai, magar purani
	// FAKE entry (jo scan me nahi thi) hat jani chahiye.
	fleetHealthCacheMu.Lock()
	_, stillFake := fleetHealthCache["https://gold-md-fake.onrender.com"]
	fleetHealthCacheMu.Unlock()
	if stillFake {
		t.Errorf("pre-existing stale cache entry survived fresh scan (%d entries)", n)
	}
}
