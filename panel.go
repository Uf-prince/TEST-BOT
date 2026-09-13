package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	goldcmds "gold-md/gold-cmds"
)

// ---------------------------------------------------------------------------
//  Multi-server configuration  (servers.json)
// ---------------------------------------------------------------------------
//  The GOLD-MD panel can run as a single front-end that fans pairing
//  requests out across up to N independent Render deployments.  Each
//  deployment is the same bot binary; the panel merely shows a
//  "SELECT SERVER" dropdown, live-checks every server, and sends the
//  pairing POST to whichever server the user picked.
//
//  servers.json is loaded once at boot.  Edit it to point at your real
//  Render URLs (no trailing slash).  maxPerServer caps how many bots a
//  single Render instance will accept before the panel tells the user to
//  switch servers.
// ---------------------------------------------------------------------------

type serverEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type serversConfig struct {
	MaxPerServer int           `json:"maxPerServer"`
	Servers      []serverEntry `json:"servers"`
}

// serverStatus is what the panel sends to the browser for each server.
type serverStatus struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Online    bool   `json:"online"`
	Sessions  int    `json:"sessions"`
	Max       int    `json:"max"`
	Full      bool   `json:"full"`
	CheckedAt string `json:"checkedAt"`
}

var (
	serversCfg     serversConfig
	serversCfgOnce sync.Once
	serversCfgErr  error
)

// loadServersConfig reads servers.json from the working directory.  It is
// called once (sync.Once) so the file is parsed a single time at boot.
// defaultServers is the fallback list used when servers.json is missing or
// fails to parse. This guarantees the panel NEVER shows "No servers
// configured" even if the file did not get copied into the container.
var defaultServers = []serverEntry{
	{Name: "SERVER 1", URL: "https://gold-md-xsvr1.onrender.com"},
	{Name: "SERVER 2", URL: "https://gold-md-xsvr2.onrender.com"},
	{Name: "SERVER 3", URL: "https://gold-md-xsvr3.onrender.com"},
	{Name: "SERVER 4", URL: "https://gold-md-xsvr4.onrender.com"},
	{Name: "SERVER 5", URL: "https://gold-md-botxd2.onrender.com"},
	{Name: "SERVER 6", URL: "https://gold-md-svrr10.onrender.com"},
	{Name: "SERVER 7", URL: "https://gold-md-svrr11.onrender.com"},
	{Name: "SERVER 8", URL: "https://gold-md-svrr12.onrender.com"},
	{Name: "SERVER 9", URL: "https://gold-md-svrr7.onrender.com"},
	{Name: "SERVER 10", URL: "https://gold-md-svrr8.onrender.com"},
	{Name: "SERVER 11", URL: "https://gold-md-svrr9.onrender.com"},
	{Name: "SERVER 12", URL: "https://gold-md-svrr13.onrender.com"},
	{Name: "SERVER 13", URL: "https://gold-md-svrr13.onrender.com"},
	{Name: "SERVER 14", URL: "https://gold-md-xsvr5.onrender.com"},
	{Name: "SERVER 15", URL: "https://gold-md-xsvr6.onrender.com"},
	{Name: "SERVER 16", URL: "https://gold-md-xsvr7.onrender.com"},
	{Name: "SERVER 17", URL: "https://gold-md-xsvr8.onrender.com"},
	{Name: "SERVER 18", URL: "https://gold-md-xsvr9.onrender.com"},
	{Name: "SERVER 19", URL: "https://gold-md-xsvr10.onrender.com"},
	{Name: "SERVER 20", URL: "https://gold-md-xsvr11.onrender.com"},
	{Name: "SERVER 21", URL: "https://gold-md-xsvr12.onrender.com"},
	{Name: "SERVER 22", URL: "https://gold-md-xsvr13.onrender.com"},
	{Name: "SERVER 23", URL: "https://gold-md-xsvr14.onrender.com"},
	{Name: "SERVER 24", URL: "https://gold-md-xsvr15.onrender.com"},
	{Name: "SERVER 25", URL: "https://gold-md-xsvr16.onrender.com"},
	{Name: "SERVER 26", URL: "https://gold-md-xsvr17.onrender.com"},
	{Name: "SERVER 27", URL: "https://gold-md-xsvr18.onrender.com"},
	{Name: "SERVER 28", URL: "https://gold-md-xsvr19.onrender.com"},
	{Name: "SERVER 29", URL: "https://gold-md-xsvr20.onrender.com"},
	{Name: "SERVER 30", URL: "https://gold-md-xsvr21.onrender.com"},
	{Name: "SERVER 31", URL: "https://gold-md-xsvr22.onrender.com"},
	{Name: "SERVER 32", URL: "https://gold-md-xsvr23.onrender.com"},
	{Name: "SERVER 33", URL: "https://gold-md-xsvr24.onrender.com"},
	{Name: "SERVER 34", URL: "https://gold-md-xsvr25.onrender.com"},
	{Name: "SERVER 35", URL: "https://gold-md-xsvr26.onrender.com"},
	{Name: "SERVER 36", URL: "https://gold-md-xsvr27.onrender.com"},
	{Name: "SERVER 37", URL: "https://gold-md-xsvr28.onrender.com"},
	{Name: "SERVER 38", URL: "https://gold-md-xsvr29.onrender.com"},
	{Name: "SERVER 39", URL: "https://gold-md-xsvr30.onrender.com"},
	{Name: "SERVER 40", URL: "https://gold-md-xsvr31.onrender.com"},
	{Name: "SERVER 41", URL: "https://gold-md-xsvr32.onrender.com"},
	{Name: "SERVER 42", URL: "https://gold-md-xsvr33.onrender.com"},
	{Name: "SERVER 43", URL: "https://gold-md-xsvr34.onrender.com"},
	{Name: "SERVER 44", URL: "https://gold-md-xsvr35.onrender.com"},
	{Name: "SERVER 45", URL: "https://gold-md-xsvr36.onrender.com"},
	{Name: "SERVER 46", URL: "https://gold-md-xsvr37.onrender.com"},
	{Name: "SERVER 47", URL: "https://gold-md-xsvr38.onrender.com"},
	{Name: "SERVER 48", URL: "https://gold-md-xsvr39.onrender.com"},
	{Name: "SERVER 49", URL: "https://gold-md-xsvr40.onrender.com"},
	{Name: "SERVER 50", URL: "https://gold-md-xsvr41.onrender.com"},
	{Name: "SERVER 51", URL: "https://gold-md-xsvr42.onrender.com"},
	{Name: "SERVER 52", URL: "https://gold-md-xsvr43.onrender.com"},
	{Name: "SERVER 53", URL: "https://gold-md-xsvr44.onrender.com"},
	{Name: "SERVER 54", URL: "https://gold-md-xsvr45.onrender.com"},
	{Name: "SERVER 55", URL: "https://gold-md-xsvr46.onrender.com"},
	{Name: "SERVER 56", URL: "https://gold-md-xsvr47.onrender.com"},
	{Name: "SERVER 57", URL: "https://gold-md-xsvr48.onrender.com"},
	{Name: "SERVER 58", URL: "https://gold-md-xsvr49.onrender.com"},
	{Name: "SERVER 59", URL: "https://gold-md-xsvr50.onrender.com"},
	{Name: "SERVER 60", URL: "https://gold-md-xsvr51.onrender.com"},
	{Name: "SERVER 61", URL: "https://gold-md-xsvr52.onrender.com"},
	{Name: "SERVER 62", URL: "https://gold-md-xsvr53.onrender.com"},
	{Name: "SERVER 63", URL: "https://gold-md-xsvr54.onrender.com"},
	{Name: "SERVER 64", URL: "https://gold-md-xsvr55.onrender.com"},
	{Name: "SERVER 65", URL: "https://gold-md-xsvr56.onrender.com"},
	{Name: "SERVER 66", URL: "https://gold-md-xsvr57.onrender.com"},
	{Name: "SERVER 67", URL: "https://gold-md-xsvr58.onrender.com"},
	{Name: "SERVER 68", URL: "https://gold-md-xsvr59.onrender.com"},
	{Name: "SERVER 69", URL: "https://gold-md-xsvr60.onrender.com"},
	{Name: "SERVER 70", URL: "https://gold-md-xsvr61.onrender.com"},
	{Name: "SERVER 71", URL: "https://gold-md-xsvr62.onrender.com"},
	{Name: "SERVER 72", URL: "https://gold-md-xsvr63.onrender.com"},
	{Name: "SERVER 73", URL: "https://gold-md-xsvr64.onrender.com"},
	{Name: "SERVER 74", URL: "https://gold-md-xsvr65.onrender.com"},
	{Name: "SERVER 75", URL: "https://gold-md-xsvr66.onrender.com"},
	{Name: "SERVER 76", URL: "https://gold-md-xsvr67.onrender.com"},
	{Name: "SERVER 77", URL: "https://gold-md-xsvr68.onrender.com"},
	{Name: "SERVER 78", URL: "https://gold-md-xsvr69.onrender.com"},
	{Name: "SERVER 79", URL: "https://gold-md-xsvr70.onrender.com"},
	{Name: "SERVER 80", URL: "https://gold-md-xsvr71.onrender.com"},
	{Name: "SERVER 81", URL: "https://gold-md-xsvr72.onrender.com"},
	{Name: "SERVER 82", URL: "https://gold-md-xsvr73.onrender.com"},
	{Name: "SERVER 83", URL: "https://gold-md-xsvr74.onrender.com"},
	{Name: "SERVER 84", URL: "https://gold-md-xsvr75.onrender.com"},
	{Name: "SERVER 85", URL: "https://gold-md-xsvr76.onrender.com"},
	{Name: "SERVER 86", URL: "https://gold-md-xsvr77.onrender.com"},
	{Name: "SERVER 87", URL: "https://gold-md-xsvr78.onrender.com"},
	{Name: "SERVER 88", URL: "https://gold-md-xsvr79.onrender.com"},
	{Name: "SERVER 89", URL: "https://gold-md-xsvr80.onrender.com"},
	{Name: "SERVER 90", URL: "https://gold-md-xsvr81.onrender.com"},
	{Name: "SERVER 91", URL: "https://gold-md-xsvr82.onrender.com"},
	{Name: "SERVER 92", URL: "https://gold-md-xsvr83.onrender.com"},
	{Name: "SERVER 93", URL: "https://gold-md-xsvr84.onrender.com"},
	{Name: "SERVER 94", URL: "https://gold-md-xsvr85.onrender.com"},
	{Name: "SERVER 95", URL: "https://gold-md-xsvr86.onrender.com"},
	{Name: "SERVER 96", URL: "https://gold-md-xsvr87.onrender.com"},
	{Name: "SERVER 97", URL: "https://gold-md-xsvr88.onrender.com"},
	{Name: "SERVER 98", URL: "https://gold-md-xsvr89.onrender.com"},
	{Name: "SERVER 99", URL: "https://gold-md-xsvr90.onrender.com"},
	{Name: "SERVER 100", URL: "https://gold-md-xsvr91.onrender.com"},
	{Name: "SERVER 101", URL: "https://gold-md-xsvr92.onrender.com"},
	{Name: "SERVER 102", URL: "https://gold-md-xsvr93.onrender.com"},
	{Name: "SERVER 103", URL: "https://gold-md-xsvr94.onrender.com"},
	{Name: "SERVER 104", URL: "https://gold-md-xsvr95.onrender.com"},
	{Name: "SERVER 105", URL: "https://gold-md-xsvr96.onrender.com"},
	{Name: "SERVER 106", URL: "https://gold-md-xsvr97.onrender.com"},
	{Name: "SERVER 107", URL: "https://gold-md-xsvr98.onrender.com"},
	{Name: "SERVER 108", URL: "https://gold-md-xsvr99.onrender.com"},
	{Name: "SERVER 109", URL: "https://gold-md-xsvr100.onrender.com"},
	{Name: "SERVER 110", URL: "https://gold-md-xsvr101.onrender.com"},
	{Name: "SERVER 111", URL: "https://gold-md-xsvr102.onrender.com"},
	{Name: "SERVER 112", URL: "https://gold-md-xsvr103.onrender.com"},
	{Name: "SERVER 113", URL: "https://gold-md-xsvr104.onrender.com"},
	{Name: "SERVER 114", URL: "https://gold-md-xsvr105.onrender.com"},
	{Name: "SERVER 115", URL: "https://gold-md-xsvr106.onrender.com"},
	{Name: "SERVER 116", URL: "https://gold-md-xsvr107.onrender.com"},
	{Name: "SERVER 117", URL: "https://gold-md-xsvr108.onrender.com"},
	{Name: "SERVER 118", URL: "https://gold-md-xsvr109.onrender.com"},
	{Name: "SERVER 119", URL: "https://gold-md-xsvr110.onrender.com"},
	{Name: "SERVER 120", URL: "https://gold-md-xsvr111.onrender.com"},
	{Name: "SERVER 121", URL: "https://gold-md-xsvr112.onrender.com"},
	{Name: "SERVER 122", URL: "https://gold-md-xsvr113.onrender.com"},
	{Name: "SERVER 123", URL: "https://gold-md-xsvr114.onrender.com"},
	{Name: "SERVER 124", URL: "https://gold-md-xsvr115.onrender.com"},
	{Name: "SERVER 125", URL: "https://gold-md-xsvr116.onrender.com"},
	{Name: "SERVER 126", URL: "https://gold-md-xsvr117.onrender.com"},
	{Name: "SERVER 127", URL: "https://gold-md-xsvr118.onrender.com"},
	{Name: "SERVER 128", URL: "https://gold-md-xsvr119.onrender.com"},
	{Name: "SERVER 129", URL: "https://gold-md-xsvr120.onrender.com"},
	{Name: "SERVER 130", URL: "https://gold-md-xsvr121.onrender.com"},
	{Name: "SERVER 131", URL: "https://gold-md-xsvr122.onrender.com"},
	{Name: "SERVER 132", URL: "https://gold-md-xsvr123.onrender.com"},
	{Name: "SERVER 133", URL: "https://gold-md-xsvr124.onrender.com"},
	{Name: "SERVER 134", URL: "https://gold-md-xsvr125.onrender.com"},
	{Name: "SERVER 135", URL: "https://gold-md-xsvr126.onrender.com"},
	{Name: "SERVER 136", URL: "https://gold-md-xsvr127.onrender.com"},
	{Name: "SERVER 137", URL: "https://gold-md-xsvr128.onrender.com"},
	{Name: "SERVER 138", URL: "https://gold-md-xsvr129.onrender.com"},
	{Name: "SERVER 139", URL: "https://gold-md-xsvr130.onrender.com"},
	{Name: "SERVER 140", URL: "https://gold-md-xsvr131.onrender.com"},
	{Name: "SERVER 141", URL: "https://gold-md-xsvr132.onrender.com"},
	{Name: "SERVER 142", URL: "https://gold-md-xsvr133.onrender.com"},
	{Name: "SERVER 143", URL: "https://gold-md-xsvr134.onrender.com"},
	{Name: "SERVER 144", URL: "https://gold-md-xsvr135.onrender.com"},
	{Name: "SERVER 145", URL: "https://gold-md-xsvr136.onrender.com"},
	{Name: "SERVER 146", URL: "https://gold-md-xsvr137.onrender.com"},
	{Name: "SERVER 147", URL: "https://gold-md-xsvr138.onrender.com"},
	{Name: "SERVER 148", URL: "https://gold-md-xsvr139.onrender.com"},
	{Name: "SERVER 149", URL: "https://gold-md-xsvr140.onrender.com"},
	{Name: "SERVER 150", URL: "https://gold-md-xsvr141.onrender.com"},
	{Name: "SERVER 151", URL: "https://gold-md-xsvr142.onrender.com"},
	{Name: "SERVER 152", URL: "https://gold-md-xsvr143.onrender.com"},
	{Name: "SERVER 153", URL: "https://gold-md-xsvr144.onrender.com"},
	{Name: "SERVER 154", URL: "https://gold-md-xsvr145.onrender.com"},
	{Name: "SERVER 155", URL: "https://gold-md-xsvr146.onrender.com"},
	{Name: "SERVER 156", URL: "https://gold-md-xsvr147.onrender.com"},
	{Name: "SERVER 157", URL: "https://gold-md-xsvr148.onrender.com"},
	{Name: "SERVER 158", URL: "https://gold-md-xsvr149.onrender.com"},
	{Name: "SERVER 159", URL: "https://gold-md-xsvr150.onrender.com"},
	{Name: "SERVER 160", URL: "https://gold-md-xsvr151.onrender.com"},
	{Name: "SERVER 161", URL: "https://gold-md-xsvr152.onrender.com"},
	{Name: "SERVER 162", URL: "https://gold-md-xsvr153.onrender.com"},
	{Name: "SERVER 163", URL: "https://gold-md-xsvr154.onrender.com"},
	{Name: "SERVER 164", URL: "https://gold-md-xsvr155.onrender.com"},
	{Name: "SERVER 165", URL: "https://gold-md-xsvr156.onrender.com"},
	{Name: "SERVER 166", URL: "https://gold-md-xsvr157.onrender.com"},
	{Name: "SERVER 167", URL: "https://gold-md-xsvr158.onrender.com"},
	{Name: "SERVER 168", URL: "https://gold-md-xsvr159.onrender.com"},
	{Name: "SERVER 169", URL: "https://gold-md-xsvr160.onrender.com"},
	{Name: "SERVER 170", URL: "https://gold-md-xsvr161.onrender.com"},
	{Name: "SERVER 171", URL: "https://gold-md-xsvr162.onrender.com"},
	{Name: "SERVER 172", URL: "https://gold-md-xsvr163.onrender.com"},
	{Name: "SERVER 173", URL: "https://gold-md-xsvr164.onrender.com"},
	{Name: "SERVER 174", URL: "https://gold-md-xsvr165.onrender.com"},
	{Name: "SERVER 175", URL: "https://gold-md-xsvr166.onrender.com"},
	{Name: "SERVER 176", URL: "https://gold-md-xsvr167.onrender.com"},
	{Name: "SERVER 177", URL: "https://gold-md-xsvr168.onrender.com"},
	{Name: "SERVER 178", URL: "https://gold-md-xsvr169.onrender.com"},
	{Name: "SERVER 179", URL: "https://gold-md-xsvr170.onrender.com"},
	{Name: "SERVER 180", URL: "https://gold-md-xsvr171.onrender.com"},
	{Name: "SERVER 181", URL: "https://gold-md-xsvr172.onrender.com"},
	{Name: "SERVER 182", URL: "https://gold-md-xsvr173.onrender.com"},
	{Name: "SERVER 183", URL: "https://gold-md-xsvr174.onrender.com"},
	{Name: "SERVER 184", URL: "https://gold-md-xsvr175.onrender.com"},
	{Name: "SERVER 185", URL: "https://gold-md-xsvr176.onrender.com"},
	{Name: "SERVER 186", URL: "https://gold-md-xsvr177.onrender.com"},
	{Name: "SERVER 187", URL: "https://gold-md-xsvr178.onrender.com"},
	{Name: "SERVER 188", URL: "https://gold-md-xsvr179.onrender.com"},
	{Name: "SERVER 189", URL: "https://gold-md-xsvr180.onrender.com"},
	{Name: "SERVER 190", URL: "https://gold-md-xsvr181.onrender.com"},
	{Name: "SERVER 191", URL: "https://gold-md-xsvr182.onrender.com"},
	{Name: "SERVER 192", URL: "https://gold-md-xsvr183.onrender.com"},
	{Name: "SERVER 193", URL: "https://gold-md-xsvr184.onrender.com"},
	{Name: "SERVER 194", URL: "https://gold-md-xsvr185.onrender.com"},
	{Name: "SERVER 195", URL: "https://gold-md-xsvr186.onrender.com"},
	{Name: "SERVER 196", URL: "https://gold-md-xsvr187.onrender.com"},
	{Name: "SERVER 197", URL: "https://gold-md-xsvr188.onrender.com"},
	{Name: "SERVER 198", URL: "https://gold-md-xsvr189.onrender.com"},
	{Name: "SERVER 199", URL: "https://gold-md-xsvr190.onrender.com"},
	{Name: "SERVER 200", URL: "https://gold-md-xsvr191.onrender.com"},
}

func loadServersConfig() {
	serversCfgOnce.Do(func() {
		serversCfg = serversConfig{MaxPerServer: 2}
		raw, err := os.ReadFile("servers.json")
		if err != nil {
			// FALLBACK: file missing (e.g. not copied in Docker image) -> use
			// built-in defaults so the panel still shows all servers.
			serversCfg.Servers = defaultServers
			serversCfg.MaxPerServer = 2
			return
		}
		if err := json.Unmarshal(raw, &serversCfg); err != nil {
			// FALLBACK: file present but invalid JSON -> use defaults.
			serversCfg.Servers = defaultServers
			serversCfg.MaxPerServer = 2
			return
		}
		if len(serversCfg.Servers) == 0 {
			// FALLBACK: servers array empty -> use defaults.
			serversCfg.Servers = defaultServers
		}
		if serversCfg.MaxPerServer <= 0 {
			serversCfg.MaxPerServer = 2
		}
	})
}

// corsMiddleware wraps an http.Handler so that every response carries
// the CORS headers needed by the multi-server panel.  The panel runs on
// one host but sends pairing POSTs directly to other Render deployments,
// so those deployments MUST allow cross-origin requests or the browser
// blocks the fetch with a CORS error.
func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, ngrok-skip-browser-warning")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// StartPanel serves the status page and WhatsApp pairing endpoints.
func StartPanel(mgr *Manager, port int) {
	loadServersConfig()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, panelHTML(mgr))
	})

	mux.HandleFunc("/health", mgr.HealthHandler)

	mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sessions := mgr.List()
		type SessInfo struct {
			JID    string `json:"jid"`
			Owner  string `json:"owner"`
			Online bool   `json:"online"`
			Prefix string `json:"prefix"`
		}
		out := make([]SessInfo, 0, len(sessions))
		for _, s := range sessions {
			// WhatsApp is truth: a session is truly online only if it is paired
			// (PairSuccess received), the websocket is connected, AND Store.ID
			// is set (device actually linked). A pending pairing (code generated
			// but never linked in WhatsApp) must NOT show as online.
			online := s.Paired && s.Client != nil && s.Client.IsConnected() &&
				s.Client.Store != nil && s.Client.Store.ID != nil
			out = append(out, SessInfo{
				JID: s.JID, Owner: s.Owner,
				Online: online,
				Prefix: s.resolvePrefix(s.JID),
			})
		}
		// count = WhatsApp-confirmed paired sessions only (pending don't count)
		_ = json.NewEncoder(w).Encode(map[string]any{"bot": "GOLD-MD", "sessions": out, "count": mgr.Count()})
	})

	// ── /reactburst : EXPERIMENT — fire N reactions from the first connected
	// session to a channel post, each with a freshly-generated message ID, to
	// test whether WhatsApp counts them as separate reactors or dedupes by
	// account. GET /reactburst?jid=<channelJID>&sid=<serverID>&n=<count>&emoji=❤️
	mux.HandleFunc("/reactburst", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		channelJIDStr := r.URL.Query().Get("jid")
		sidStr := r.URL.Query().Get("sid")
		nStr := r.URL.Query().Get("n")
		emoji := r.URL.Query().Get("emoji")
		if emoji == "" {
			emoji = "❤️"
		}
		if channelJIDStr == "" || sidStr == "" || nStr == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "need jid, sid, n (and optional emoji)"})
			return
		}
		channelJID, err := types.ParseJID(channelJIDStr)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad jid: " + err.Error()})
			return
		}
		var sid int
		fmt.Sscanf(sidStr, "%d", &sid)
		var n int
		fmt.Sscanf(nStr, "%d", &n)
		if n <= 0 || n > 500 {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "n must be 1..500"})
			return
		}
		// find first connected session
		var cli *whatsmeow.Client
		for _, s := range mgr.List() {
			if s.Client != nil && s.Client.IsConnected() {
				cli = s.Client
				break
			}
		}
		if cli == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no connected session"})
			return
		}
		results := make([]map[string]any, 0, n)
		ok := 0
		for i := 0; i < n; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := cli.NewsletterSendReaction(ctx, channelJID, types.MessageServerID(sid), emoji, "")
			cancel()
			step := map[string]any{"i": i, "ok": err == nil}
			if err != nil {
				step["error"] = err.Error()
			} else {
				ok++
			}
			results = append(results, step)
			time.Sleep(700 * time.Millisecond)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channelJID": channelJIDStr,
			"serverID":   sid,
			"n":          n,
			"emoji":      emoji,
			"sentFrom":   cli.Store.ID.String(),
			"ok":         ok,
			"results":    results,
		})
	})

	// ── /ppdiag : .PP 406 debugger — runs the full smart profile-picture
	// ladder on a local image file from curl, no WhatsApp message needed.
	// GET /ppdiag?img=/tmp/pptest.jpg  (optional &jid=<own PN>)
	mux.HandleFunc("/ppdiag", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		imgPath := r.URL.Query().Get("img")
		if imgPath == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "need img=<local file path> (optional jid=<PN>)"})
			return
		}
		data, err := os.ReadFile(imgPath)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "read img: " + err.Error()})
			return
		}
		var cli *whatsmeow.Client
		for _, s := range mgr.List() {
			if s.Client != nil && s.Client.IsConnected() {
				cli = s.Client
				break
			}
		}
		if cli == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no connected session"})
			return
		}
		jidStr := r.URL.Query().Get("jid")
		if jidStr == "" && cli.Store != nil && cli.Store.ID != nil {
			jidStr = cli.Store.ID.ToNonAD().String()
		}
		start := time.Now()
		err = goldcmds.RunPPLadder(cli, data, jidStr)
		resp := map[string]any{
			"img":     imgPath,
			"bytes":   len(data),
			"jid":     jidStr,
			"elapsed": time.Since(start).String(),
			"ok":      err == nil,
		}
		if err != nil {
			resp["error"] = err.Error()
			resp["hint"] = "full ladder trace: grep PP_DEBUG 11239_gold-md-bot.out.log"
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// ── /ppcheck : verify the bot's CURRENT profile picture — GETs the pic
	// info for the bot's own JID and downloads the image to /tmp/pp_current.jpg
	// so it can be visually checked (curl + see the file).
	// ── /ppraw : server-rule probe — sends ONE set IQ with a custom-encoded
	// payload. GET /ppraw?img=<path>&q=<quality>&maxside=<n>  (q default 90,
	// maxside 0 = original dimensions). Returns dims/bytes/accepted — used
	// to discover the REAL server acceptance rule (aspect vs side vs size).
	mux.HandleFunc("/ppraw", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		imgPath := r.URL.Query().Get("img")
		if imgPath == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "need img=<local path> (&q=, &maxside=)"})
			return
		}
		data, err := os.ReadFile(imgPath)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "read img: " + err.Error()})
			return
		}
		var cli *whatsmeow.Client
		for _, s := range mgr.List() {
			if s.Client != nil && s.Client.IsConnected() {
				cli = s.Client
				break
			}
		}
		if cli == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no connected session"})
			return
		}
		q := 90
		fmt.Sscanf(r.URL.Query().Get("q"), "%d", &q)
		maxside := 0
		fmt.Sscanf(r.URL.Query().Get("maxside"), "%d", &maxside)
		start := time.Now()
		dims, nbytes, serr := goldcmds.PPRawTest(cli, data, maxside, q)
		resp := map[string]any{
			"img":     imgPath,
			"dims":    dims,
			"bytes":   nbytes,
			"quality": q,
			"maxside": maxside,
			"elapsed": time.Since(start).String(),
			"ok":      serr == nil,
		}
		if serr != nil {
			resp["error"] = serr.Error()
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/ppcheck", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var cli *whatsmeow.Client
		for _, s := range mgr.List() {
			if s.Client != nil && s.Client.IsConnected() {
				cli = s.Client
				break
			}
		}
		if cli == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no connected session"})
			return
		}
		if cli.Store == nil || cli.Store.ID == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no stored JID"})
			return
		}
		own := cli.Store.ID.ToNonAD()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := cli.GetProfilePictureInfo(ctx, own, nil)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "get pic info: " + err.Error(), "jid": own.String()})
			return
		}
		if info == nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "no profile picture set (info nil)", "jid": own.String()})
			return
		}
		resp := map[string]any{
			"jid":         own.String(),
			"picID":       info.ID,
			"type":        info.Type,
			"url":         info.URL,
			"directPath":  info.DirectPath,
		}
		// download the pic so it can be viewed
		if info.URL != "" {
			dlCtx, dlCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer dlCancel()
			req, rerr := http.NewRequestWithContext(dlCtx, http.MethodGet, info.URL, nil)
			if rerr == nil {
				c := &http.Client{Timeout: 25 * time.Second}
				if hres, herr := c.Do(req); herr == nil {
					defer hres.Body.Close()
					if body, berr := io.ReadAll(hres.Body); berr == nil && len(body) > 0 {
					out := "/tmp/pp_current.jpg"
				if werr := os.WriteFile(out, body, 0644); werr == nil {
						resp["downloaded"] = out
						resp["bytes"] = len(body)
					}
				}
			}
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// ── /api/servers : returns live status of every configured server ──
	// The browser polls this to populate the SELECT SERVER dropdown with
	// 🟢/🔴 indicators and to grey-out full servers.
	mux.HandleFunc("/api/servers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if serversCfgErr != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bot":     "GOLD-MD",
				"error":   "servers.json not loaded: " + serversCfgErr.Error(),
				"servers": []serverStatus{},
				"max":     3,
			})
			return
		}
		statuses := checkAllServers(serversCfg)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"bot":     "GOLD-MD",
			"max":     serversCfg.MaxPerServer,
			"servers": statuses,
		})
	})

	// /code?phone=923xxxxxxxxx  —  direct pairing via simple GET (browser/curl).
	// Returns JSON with the pairing code so a user can open a link in a
	// browser and get the code without the panel UI. This is the "direct
	// pairing" link: max 1 session on THIS local instance.
	mux.HandleFunc("/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		phone := strings.Map(func(rn rune) rune {
			if rn >= '0' && rn <= '9' {
				return rn
			}
			return -1
		}, strings.TrimSpace(r.URL.Query().Get("phone")))
		if phone == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "add ?phone=<digits> with country code"})
			return
		}
		if mgr.AlreadyConnected(phone) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "already_connected",
				"message": "BOT ALREADY CONNECTED AND WORKING WELL",
				"jid":     normalizeJID(phone),
			})
			return
		}
		if mgr.Count() >= 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "max_pairing_reached",
				"error":  "MAX PAIRING REQUEST REACHED — direct instance already has 1 session",
				"count":  mgr.Count(), "max": 1,
			})
			return
		}
		code, err := mgr.PairWithCode(phone)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "error": err.Error(), "count": mgr.Count(), "max": 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "pairing",
			"jid":     normalizeJID(phone),
			"code":    code,
			"message": "Enter this code in WhatsApp > Linked Devices > Link with phone number instead",
		})
	})

	mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Phone string `json:"phone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON body"})
			return
		}

		// Accept only digits so Android opens the numeric keyboard and WhatsApp
		// always receives a normalized country-code number without a plus sign.
		phone := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, strings.TrimSpace(body.Phone))
		if phone == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "enter digits-only phone number with country code"})
			return
		}

		if mgr.AlreadyConnected(phone) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":  "already_connected",
				"message": "BOT ALREADY CONNECTED AND WORKING WELL",
				"jid":     normalizeJID(phone),
			})
			return
		}

		// Effective pairing limit.
		//   - Direct pairing (no server selected → /pair?direct=1): max 1 session.
		//     This is the local/tunnel single-instance panel — only the owner
		//     should pair here.
		//   - Server-selected pairing (dropdown → remote server's /pair): uses
		//     GOLDMD_MAX_SESSIONS (default 3 via start_bot.sh). This caps how
		//     many users each Render instance accepts.
		isDirect := r.URL.Query().Get("direct") == "1"
		var effectiveMax int
		if isDirect {
			effectiveMax = 1
		} else {
			effectiveMax = maxPairedSessions()
		}

		if mgr.Count() >= effectiveMax {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "max_pairing_reached",
				"error":  "MAX PAIRING REQUEST REACHED GO BACK AND SWITCH TO ANOTHER SERVER AND TRY AGAIN",
				"count":  mgr.Count(), "max": effectiveMax,
			})
			return
		}

		code, err := mgr.PairWithCode(phone)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "error": err.Error(), "count": mgr.Count(), "max": effectiveMax})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "pairing", "jid": normalizeJID(phone), "code": code,
			"message": "Enter this code in WhatsApp > Linked Devices",
		})
	})

	mux.HandleFunc("/pair/qr", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		tempJID := fmt.Sprintf("pending-%d@s.whatsapp.net", len(mgr.List()))
		if err := mgr.StartSession(tempJID); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "qr_generated", "temp_jid": tempJID})
	})

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), corsMiddleware(mux)); err != nil {
		ErrLog("Panel server error: %v", err)
	}
}

// checkAllServers hits /health on every configured server in parallel and
// returns a slice of serverStatus ready for the browser.  A server is
// "online" only if /health returns 200 with a parseable sessions count.
// "full" is true when sessions >= maxPerServer.
func checkAllServers(cfg serversConfig) []serverStatus {
	if len(cfg.Servers) == 0 {
		return []serverStatus{}
	}
	statuses := make([]serverStatus, len(cfg.Servers))
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 6 * time.Second}

	for i, srv := range cfg.Servers {
		wg.Add(1)
		go func(idx int, s serverEntry) {
			defer wg.Done()
			st := serverStatus{
				Name: s.Name,
				URL:  s.URL,
				Max:  cfg.MaxPerServer,
			}
			healthURL := strings.TrimRight(s.URL, "/") + "/health"
			resp, err := client.Get(healthURL)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var hr struct {
						Sessions int `json:"sessions"`
					}
					if jerr := json.NewDecoder(resp.Body).Decode(&hr); jerr == nil {
						st.Online = true
						st.Sessions = hr.Sessions
						st.Full = hr.Sessions >= cfg.MaxPerServer
					}
				}
			}
			st.CheckedAt = time.Now().UTC().Format(time.RFC3339)
			statuses[idx] = st
		}(i, srv)
	}
	wg.Wait()
	return statuses
}

// ---------------------------------------------------------------------------
//  Panel HTML  —  SELECT SERVER + live status + pairing
// ---------------------------------------------------------------------------
//  The page polls /api/servers, builds the dropdown with 🟢/🔴 and the
//  per-server count (e.g. "SERVER 03  🟢 2/3").  When the user clicks
//  PAIR, the JS POSTs {phone} directly to the selected server's /pair
//  endpoint (client-side fetch — no proxy needed) and renders the code
//  or the MAX PAIRING REACHED message.
// ---------------------------------------------------------------------------

func panelHTML(mgr *Manager) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>GOLD-MD · Control Panel</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}body{font-family:system-ui,sans-serif;background:#f2f3f5;color:#000;font-weight:700;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px;padding-bottom:75px}.card{background:#ffffff;border:1px solid #d7dade;border-radius:16px;padding:32px;max-width:640px;width:100%%;box-shadow:0 2px 10px rgba(0,0,0,.06)}h1{color:#000;font-size:28px;margin-bottom:8px;text-align:center;font-weight:800}.sub{color:#000;text-align:center;margin-bottom:28px;font-size:14px;font-weight:700}.status{background:#f2f3f5;border-radius:10px;padding:16px;margin-bottom:24px;display:flex;justify-content:space-between;align-items:center}.dot{width:10px;height:10px;border-radius:50%%;background:#2ea043;display:inline-block;margin-right:8px}.badge{background:#2ea043;color:#fff;padding:4px 12px;border-radius:20px;font-size:13px;font-weight:800}label{display:block;margin-bottom:6px;color:#000;font-size:14px;font-weight:800;margin-top:16px}select,input{width:100%%;padding:13px 14px;background:#fff;border:1px solid #b9bec5;border-radius:8px;color:#000;font-weight:700;font-size:16px;margin-bottom:16px}#server,#phone{border:3px solid #c22a2a}select:focus,input:focus{outline:none;border-color:#c9a300}.btn{width:100%%;padding:14px;background:#ffd700;color:#000;border:3px solid #c22a2a;border-radius:8px;font-size:15px;font-weight:800;cursor:pointer}.btn:disabled{opacity:.5;cursor:not-allowed}.videoBtn{position:fixed;left:20px;right:20px;bottom:16px;width:auto;margin:0;padding:14px;background:#ffd700;color:#000;border:4px solid #c22a2a;border-radius:10px;font-size:14px;font-weight:800;cursor:pointer;text-align:center;z-index:999;box-shadow:0 2px 10px rgba(0,0,0,.15)}.videoBtn:hover{background:#ffde33}.result{margin-top:16px;padding:18px;background:#f2f3f5;border-radius:8px;font-size:14px;font-weight:700;display:none;text-align:center}.pair-code{display:block;margin:14px auto;padding:14px 10px;background:#e7f7ec;border:3px solid #c22a2a;border-radius:12px;color:#177a34;font-weight:800;font-size:20px;letter-spacing:5px;cursor:pointer;user-select:none;text-align:center;max-width:220px}.hint{color:#000;font-size:12px;font-weight:700}.error{color:#c22a2a}.warn{color:#a3690a}.links{margin-top:24px;text-align:center;font-size:13px;font-weight:700}.links a{color:#0b5fcc;text-decoration:none;margin:0 8px;font-weight:800}
</style></head><body><div class="card">
<h1>🔰 GOLD-MD 🔰</h1>
<label for="server" style="text-align:center;font-size:18px;font-weight:900">SELECT SERVER TO PAIR</label>
<select id="server"><option value="">Loading servers…</option></select>

<label for="phone" style="text-align:center">TYPE YOUR NUMBER HERE IN THIS BOX</label>
<input id="phone" type="tel" inputmode="numeric" pattern="[0-9]*" autocomplete="tel" placeholder="e.g. 923xxxxxxxxx" oninput="this.value=this.value.replace(/[^0-9]/g,'')">
<button class="btn" id="pairBtn" onclick="pair()">GET PAIR CODE</button>
<div class="result" id="result"></div>
</div>
<div class="videoBtn" id="videoBtn" onclick="openVideoBox()">I NEED THE BOT CREATING VIDEO</div>
<script>
let SERVERS=[],MAXP=10,sel=null;
function openVideoBox(){
  const link='https://youtu.be/w4a_3wYUMr0?si=nUrgD6IvUMrFoz_S9';
  window.open(link,'_blank');
}
async function loadServers(){
  try{
    const r=await fetch('/api/servers'),d=await r.json();
    SERVERS=d.servers||[];MAXP=d.max||10;sel=document.getElementById('server');
    if(!SERVERS.length){sel.innerHTML='<option value="">No servers configured (edit servers.json)</option>';return;}
    const prev=sel?sel.value:'';
    let opts='<option value="">— Select a server —</option>';
    SERVERS.forEach(s=>{
      const icon=s.online?'🟢':'🔴';
      const tag=s.full?' — FULL '+s.sessions+'/'+s.max:(s.online?' — online '+s.sessions+'/'+s.max:' — offline');
      opts+='<option value="'+s.url+'" '+(s.full?'disabled':'')+'>'+icon+' '+s.name+tag+'</option>';
    });
    sel.innerHTML=opts;
    let restored=false;
    for(let i=0;i<sel.options.length;i++){if(sel.options[i].value===prev&&!sel.options[i].disabled){sel.selectedIndex=i;restored=true;break;}}
    if(!restored){sel.selectedIndex=0;}
  }catch(e){document.getElementById('server').innerHTML='<option value="">Failed to load servers</option>';}
}
async function pair(){
  const phone=document.getElementById('phone').value.trim(),
        srv=document.getElementById('server').value,
        res=document.getElementById('result');
  if(!phone){res.className='result error';res.style.display='block';res.textContent='Enter your phone number first';return;}
  if(!srv){res.className='result error';res.style.display='block';res.innerHTML='\u26d4 <strong>Select any free server first</strong><br><div class="hint">Pick a \ud83d\udfe2 green server from the dropdown above, then pair.</div>';return;}
  const url=(srv.endsWith('/')?srv.slice(0,-1):srv)+'/pair';
  res.className='result';res.style.display='block';res.textContent='PLEASE WAIT.......';
  try{
    const r=await fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({phone})}),
          d=await r.json();
    if(d.status==='max_pairing_reached'||d.status==='error'){
      res.className='result error';res.style.display='block';
      res.innerHTML='⛔ <strong>MAX PAIRING REACHED</strong><br>TRY ANOTHER SERVER<br><div class="hint">Go back, pick a different 🟢 server from the dropdown and try again.</div>';
      loadServers();return;
    }
    if(!r.ok||d.error){res.className='result error';res.textContent=d.error||('Server error ('+r.status+')');return;}
    if(d.status==='already_connected'){res.className='result';res.style.display='block';res.innerHTML='✅ <strong>BOT ALREADY CONNECTED AND WORKING WELL</strong><div class="hint">Redis session found. Pairing dobara zaroori nahi.</div>';return;}
    res.innerHTML='CLICK TO COPY CODE<button class="pair-code" id="pairCode" data-code="'+d.code+'" onclick="copyCode()">'+d.code+'</button><div class="hint">WhatsApp → Linked Devices → Link with phone number instead</div>';
    loadServers();
  }catch(e){res.className='result error';res.style.display='block';res.textContent='Request failed: '+e.message+' — server may be stopped. Try another 🟢 server.';}
}
async function copyCode(){const el=document.getElementById('pairCode');try{await navigator.clipboard.writeText(el.textContent);el.textContent='COPIED';setTimeout(()=>{el.textContent=el.dataset.code||''},1200)}catch(e){const range=document.createRange();range.selectNodeContents(el);getSelection().removeAllRanges();getSelection().addRange(range)}}
loadServers();setInterval(loadServers,15000);
</script></body></html>`)
}
