package main

import (
	"os"
	"testing"
)

// TestReplyCacheLineLevelRoundTrip: ek line translate ho kar RAM me cache ho,
// dobara lookup pe wahi mile — bina translator chhue.
func TestReplyCacheLineLevelRoundTrip(t *testing.T) {
	rcInit()
	botJID, lang := "923158930864@s.whatsapp.net", "ur"
	line := "THE BOT IS ONLINE AND WORKING"

	if _, ok := rcGet(botJID, lang, line); ok {
		t.Fatalf("cold cache pe hit mila — expected miss")
	}
	rcPut(botJID, lang, line, "بوٹ آن لائن ہے")
	got, ok := rcGet(botJID, lang, line)
	if !ok || got != "بوٹ آن لائن ہے" {
		t.Fatalf("cache hit expected, got (%q, %v)", got, ok)
	}
}

// TestReplyCachePersistsToDiskAndReloads: RAM saaf karne ke baad bhi disk se
// wapas aa jaye (restart simulation).
func TestReplyCachePersistsToDiskAndReloads(t *testing.T) {
	rcInit()
	botJID, lang := "923001112222@s.whatsapp.net", "hi"
	line := "BOT IS ONLINE"
	rcPut(botJID, lang, line, "बॉट ऑनलाइन है")
	rcFlush(botJID + "\x00" + lang)

	if _, err := os.Stat(rcFile(botJID, lang)); err != nil {
		t.Fatalf("disk file nahi bana: %v", err)
	}
	// Simulate restart: RAM khaali, hot flag hata do.
	rcMu.Lock()
	rcRAM = map[string]string{}
	rcHot = map[string]bool{}
	rcMu.Unlock()

	got, ok := rcGet(botJID, lang, line)
	if !ok || got != "बॉट ऑनलाइन है" {
		t.Fatalf("disk se reload expected, got (%q, %v)", got, ok)
	}
}

// TestReplyCacheNeverCachesIdentityOrEmpty: identity/empty translations ko cache
// nahi karna chahiye (warna cache bekaar bhar jaye).
func TestReplyCacheNeverCachesIdentityOrEmpty(t *testing.T) {
	rcInit()
	botJID, lang := "923004445555@s.whatsapp.net", "ar"
	rcPut(botJID, lang, "SAME", "SAME")
	rcPut(botJID, lang, "EMPTY", "")
	if _, ok := rcGet(botJID, lang, "SAME"); ok {
		t.Fatalf("identity translation cache nahi honi chahiye")
	}
	if _, ok := rcGet(botJID, lang, "EMPTY"); ok {
		t.Fatalf("empty translation cache nahi honi chahiye")
	}
}

// TestReplyCachePerLanguageIsolation: ek hi line ka har language ka apna entry.
func TestReplyCachePerLanguageIsolation(t *testing.T) {
	rcInit()
	botJID := "923006667777@s.whatsapp.net"
	line := "MENU"
	rcPut(botJID, "ur", line, "مینو")
	rcPut(botJID, "hi", line, "मेनू")

	if v, ok := rcGet(botJID, "ur", line); !ok || v != "مینو" {
		t.Fatalf("ur entry wrong: (%q, %v)", v, ok)
	}
	if v, ok := rcGet(botJID, "hi", line); !ok || v != "मेनू" {
		t.Fatalf("hi entry wrong: (%q, %v)", v, ok)
	}
	if _, ok := rcGet(botJID, "fr", line); ok {
		t.Fatalf("fr ka entry nahi hona chahiye")
	}
}
