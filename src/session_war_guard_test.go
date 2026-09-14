package main

import (
	"os"
	"strings"
	"testing"
)

// stripComments: Go source se line + block comments hata deta hai, taaki
// forbidden-call scan sirf real code pe chale (mechanism-explain comments
// false positive na dein).
func stripComments(src string) string {
	var out []byte
	inLine, inBlock, inStr := false, false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inLine {
			if c == '\n' {
				inLine = false
				out = append(out, c)
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				inBlock = false
				i++
			}
			continue
		}
		if inStr {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				out = append(out, src[i+1])
				i++
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch {
		case c == '"' && (len(out) == 0 || out[len(out)-1] != '\''):
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			inLine = true
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			inBlock = true
			i++
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// caseBody: event-router case clause ka body — next "case *events." tak.
// mergedRegion: root consolidation ke baad session_guards.go me har original
// file ka content apne separator comment (merged from <file>) ke baad next
// separator tak exact rehta hai — tests usi region ko scan karte hain,
// taaki doosri merged files ka legit code (cleanupSession etc.) war-guard
// leak na lage.
func mergedRegion(src, orig string) string {
	marker := "(merged from " + orig + ")"
	start := strings.Index(src, marker)
	if start < 0 {
		return src
	}
	rest := src[start:]
	next := strings.Index(rest[len(marker):], "\n// \u2550")
	if next < 0 {
		return rest
	}
	return rest[:len(marker)+next]
}

func caseBody(src string, marker string) string {
	i := strings.Index(src, marker)
	if i < 0 {
		return ""
	}
	rest := src[i:]
	j := strings.Index(rest[len(marker):], "\ncase *events.")
	if j < 0 {
		return rest
	}
	return rest[:len(marker)+j]
}

// ============================================================================
// SESSION WAR GUARD — structure tests (owner order):
// "kisi doosre server se session reconnect aye to bot use bhar me bhej de —
// apne session me milne hi na de, q k wo already chal rha hai. Bar bar
// reconnect → crash → 401 → purge ka war loop yahin khatam."
// ============================================================================

// TestWarGuardFileStructure: session_guards.go ki zaroori cheezein.
func TestWarGuardFileStructure(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal("session_guards.go missing:", err)
	}
	src := mergedRegion(string(b), "session_war_guard.go")

	for _, want := range []string{
		"func (m *Manager) surrenderSession(s *Session, reason string)",
		"func (m *Manager) warRetake(s *Session)",
		"func (m *Manager) handleStreamReplaced(s *Session)",
		"func (m *Manager) warGuardShouldSkipConnect(jid string) bool",
		"fleetHeldByLiveServer(jid)",
		"warRetakeDelay",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("session_guards.go me missing: %q", want)
		}
	}

	// Surrender must NOT touch Storj blob / registry / claims — other
	// live server owns them; WhatsApp ne logout bola hi nahi.
	// (scan comment-free code pe — mechanism-explain comments me in
	// naamon ka zikr hota hai, wo leak nahi hai.)
	code := stripComments(src)
	for _, forbidden := range []string{
		"cleanupSession(",
		"fleetSaveBlob(",
		"fleetRemoveBlob(",
		"setRem(",
		"DelSetting(",
	} {
		if strings.Contains(code, forbidden) {
			t.Errorf("surrender/war-guard me purge leak: %q", forbidden)
		}
	}
}

// TestWarGuardHookStreamReplaced: manager.go event router me StreamReplaced
// ab khali nahi — handleStreamReplaced call hona chahiye.
func TestWarGuardHookStreamReplaced(t *testing.T) {
	b, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatal("manager.go missing:", err)
	}
	src := string(b)

	body := caseBody(src, "case *events.StreamReplaced:")
	if body == "" {
		t.Fatal("manager.go me StreamReplaced case missing")
	}
	if !strings.Contains(stripComments(body), "s.Manager.handleStreamReplaced(s)") {
		t.Error("StreamReplaced case handleStreamReplaced call nahi karta")
	}

	// LoggedOut/ConnectFailure handlers ko war-guard ne touch NAHI kiya
	// (WhatsApp-side logout → purge wala rasta bilkul same rahe).
	loggedOutBody := caseBody(src, "case *events.LoggedOut:")
	if loggedOutBody == "" {
		t.Fatal("manager.go me LoggedOut case missing")
	}
	if !strings.Contains(stripComments(loggedOutBody), "cleanupSession") {
		t.Error("LoggedOut handler cleanupSession kho gaya — logout rasta change hua")
	}
}

// TestWarGuardHookWatchdog: doReconnect + handleDisconnectedEvent me
// pre-Connect war guard lagna chahiye.
func TestWarGuardHookWatchdog(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal("session_guards.go missing:", err)
	}
	src := string(b)

	// doReconnect function body me guard.
	i := strings.Index(src, "func (m *Manager) doReconnect")
	if i < 0 {
		t.Fatal("doReconnect missing")
	}
	end := strings.Index(src[i:], "\n}\n")
	if end < 0 {
		end = 2000
	}
	body := src[i : i+end]
	if !strings.Contains(body, "warGuardShouldSkipConnect") {
		t.Error("doReconnect me warGuardShouldSkipConnect guard missing")
	}

	// handleDisconnectedEvent function body me guard.
	j := strings.Index(src, "func (m *Manager) handleDisconnectedEvent")
	if j < 0 {
		t.Fatal("handleDisconnectedEvent missing")
	}
	end2 := strings.Index(src[j:], "\n}\n")
	if end2 < 0 {
		end2 = 2000
	}
	body2 := src[j : j+end2]
	if !strings.Contains(body2, "warGuardShouldSkipConnect") {
		t.Error("handleDisconnectedEvent me warGuardShouldSkipConnect guard missing")
	}
}

// TestWarGuardSurrenderSafeOrder: surrender ka local cleanup order —
// releaseSlot pehle, phir delete(m.sessions), phir Disconnect.
func TestWarGuardSurrenderSafeOrder(t *testing.T) {
	b, err := os.ReadFile("session_guards.go")
	if err != nil {
		t.Fatal("session_guards.go missing:", err)
	}
	src := mergedRegion(string(b), "session_war_guard.go")

	i := strings.Index(src, "func (m *Manager) surrenderSession")
	if i < 0 {
		t.Fatal("surrenderSession missing")
	}
	end := strings.Index(src[i:], "\n}\n")
	if end < 0 {
		end = 1500
	}
	body := src[i : i+end]

	for _, want := range []string{
		"m.releaseSlot(s.JID)",
		"delete(m.sessions, s.JID)",
		"s.Client.Disconnect()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("surrenderSession me missing: %q", want)
		}
	}
	// Storj purge function cleanupSession ka naam surrender ke andar na ho.
	if strings.Contains(body, "cleanupSession") {
		t.Error("surrenderSession cleanupSession call karta hai — leak!")
	}
}
