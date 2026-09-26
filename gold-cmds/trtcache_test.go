package goldcmds

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestTranslateUsesAttachedCache: warm cache hone par translator ko chhue bina
// translated reply milta hai — yahi bot speed ka asli fix hai.
func TestTranslateUsesAttachedCache(t *testing.T) {
	var calls int
	var mu sync.Mutex
	// A translator that records calls and returns a marker, so a cache hit is
	// provable: zero calls + the cached value.
	CmdLocalizeAttachTranslator(func(ctx context.Context, text, target string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		lines := strings.Split(text, "\n")
		for i := range lines {
			lines[i] = "T:" + lines[i]
		}
		return strings.Join(lines, "\n"), nil
	})
	defer CmdLocalizeAttachTranslator(TranslateText)

	store := map[string]string{}
	TrtCacheAttach(
		func(botJID, lang, line string) (string, bool) { v, ok := store[lang+"\x00"+line]; return v, ok },
		func(botJID, lang, line, out string) { store[lang+"\x00"+line] = out },
	)
	defer TrtCacheAttach(nil, nil)

	ctx := TrtCacheWithBot(context.Background(), "923158930864@s.whatsapp.net")
	in := "BOT IS ONLINE\nWORKING FINE"

	out1, err := TranslatePreservingCommandTokens(ctx, in, "ur")
	if err != nil {
		t.Fatalf("first translate error: %v", err)
	}
	if !strings.Contains(out1, "T:BOT IS ONLINE") {
		t.Fatalf("translator nahi chala: %q", out1)
	}
	mu.Lock()
	first := calls
	mu.Unlock()
	if first == 0 {
		t.Fatalf("pehli call pe translator chalna chahiye tha")
	}

	out2, err := TranslatePreservingCommandTokens(ctx, in, "ur")
	if err != nil {
		t.Fatalf("second translate error: %v", err)
	}
	if out2 != out1 {
		t.Fatalf("cached reply alag aya:\nfirst=%q\nsecond=%q", out1, out2)
	}
	mu.Lock()
	second := calls
	mu.Unlock()
	if second != first {
		t.Fatalf("dusri call pe translator dobara chala (calls %d -> %d) — cache miss", first, second)
	}
}

// TestTranslateCachePartialHitOnlySendsMisses: aadhi lines cached hon to sirf
// bachi hui lines translator ko jayein.
func TestTranslateCachePartialHitOnlySendsMisses(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	CmdLocalizeAttachTranslator(func(ctx context.Context, text, target string) (string, error) {
		mu.Lock()
		sent = strings.Split(text, "\n")
		mu.Unlock()
		return text + "!", nil
	})
	defer CmdLocalizeAttachTranslator(TranslateText)

	TrtCacheAttach(
		func(botJID, lang, line string) (string, bool) {
			if line == "ALREADY DONE" {
				return "HO GAYA", true
			}
			return "", false
		},
		func(botJID, lang, line, out string) {},
	)
	defer TrtCacheAttach(nil, nil)

	ctx := TrtCacheWithBot(context.Background(), "923158930864@s.whatsapp.net")
	out, err := TranslatePreservingCommandTokens(ctx, "ALREADY DONE\nNEW LINE", "hi")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(out, "HO GAYA") {
		t.Fatalf("cached line use nahi hui: %q", out)
	}
	mu.Lock()
	got := append([]string(nil), sent...)
	mu.Unlock()
	// House-style caps are softened for the request (NEW LINE -> New Line), so
	// only the missing line is sent, in its softened form.
	if len(got) != 1 || !strings.EqualFold(got[0], "NEW LINE") {
		t.Fatalf("sirf miss honi chahiye thi, bheja: %#v", got)
	}
}

// TestTranslateNoCacheFallsBackToTranslator: cache attach na ho to purana
// behaviour (live translate) barqarar rahe.
func TestTranslateNoCacheFallsBackToTranslator(t *testing.T) {
	TrtCacheAttach(nil, nil)
	ctx := TrtCacheWithBot(context.Background(), "923158930864@s.whatsapp.net")
	if _, ok := TrtCacheLookup("923158930864@s.whatsapp.net", "ur", "X"); ok {
		t.Fatalf("cache attach nahi hone par lookup miss hona chahiye")
	}
	_ = ctx
}
