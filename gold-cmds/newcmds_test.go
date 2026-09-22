package goldcmds

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestTRTLangResolution verifies the language index resolves codes + names.
func TestTRTLangResolution(t *testing.T) {
	cases := map[string]string{
		"ur":      "ur",
		"UR":      "ur",
		"urdu":    "ur",
		"URDU":    "ur",
		"en":      "en",
		"english": "en",
		"chinese": "zh-CN",
		"farsi":   "fa",
		"tagalog": "fil",
		"hi":      "hi",
		"hindi":   "hi",
	}
	for in, want := range cases {
		got, ok := trtResolveLang(in)
		if !ok || got != want {
			t.Errorf("trtResolveLang(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	if _, ok := trtResolveLang("notalang"); ok {
		t.Errorf("trtResolveLang(notalang) should be false")
	}
}

// TestTRTSplitArgs verifies language token extraction.
func TestTRTSplitArgs(t *testing.T) {
	lang, text, had := trtSplitArgs([]string{"ur", "hello", "brother"})
	if !had || lang != "ur" || text != "hello brother" {
		t.Errorf("split with lang: got %q %q %v", lang, text, had)
	}
	lang, text, had = trtSplitArgs([]string{"hello", "brother"})
	if had || lang != "" || text != "hello brother" {
		t.Errorf("split no lang: got %q %q %v", lang, text, had)
	}
}

// TestTRTGuideListsLanguages ensures the guidance lists every language.
func TestTRTGuideListsLanguages(t *testing.T) {
	g := trtGuide(".")
	if !strings.Contains(g, "TRANSLATOR") {
		t.Errorf("guide missing title")
	}
	for _, l := range trtLangs {
		if !strings.Contains(g, l.Code) {
			t.Errorf("guide missing lang code %q", l.Code)
		}
	}
	if len(trtLangs) < 100 {
		t.Errorf("expected 100+ languages, got %d", len(trtLangs))
	}
}

// TestTTSChunk verifies chunking respects the 190-rune limit.
func TestTTSChunk(t *testing.T) {
	short := "hello world"
	if c := ttsChunk(short, 190); len(c) != 1 || c[0] != short {
		t.Errorf("short chunk: %v", c)
	}
	long := strings.Repeat("word ", 100) // 500 chars
	chunks := ttsChunk(long, 190)
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > 190 {
			t.Errorf("chunk %d too long: %d runes", i, len([]rune(c)))
		}
	}
}

// TestATTPEscape verifies ffmpeg drawtext escaping.
func TestATTPEscape(t *testing.T) {
	got := attpEscape("a:b'c%d,e[f]")
	for _, ch := range []string{":", "'", "%", ",", "[", "]"} {
		if !strings.Contains(got, "\\"+ch) {
			t.Errorf("escape did not backslash-escape %q in %q", ch, got)
		}
	}
}

// TestATTPBuildSticker builds a real sticker and checks it is a valid WebP.
func TestATTPBuildSticker(t *testing.T) {
	if !isFfmpegAvailable() {
		t.Skip("ffmpeg not available")
	}
	path, err := attpBuildSticker(context.Background(), "UMAR")
	if err != nil {
		t.Fatalf("attpBuildSticker: %v", err)
	}
	defer os.Remove(path)
	st, err := os.Stat(path)
	if err != nil || st.Size() < 512 {
		t.Fatalf("sticker too small: %v", err)
	}
	data, _ := os.ReadFile(path)
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Errorf("not a valid WebP (RIFF/WEBP header missing)")
	}
	t.Logf("sticker built: %d bytes", st.Size())
}

// TestTRTLiveTranslate hits the real Google Translate endpoint.
func TestTRTLiveTranslate(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") == "" {
		t.Skip("set GOLDMD_LIVE=1 to run live network test")
	}
	out, det, err := trtTranslate(context.Background(), "Good morning my friend", "ur")
	if err != nil {
		t.Fatalf("trtTranslate: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("empty translation")
	}
	t.Logf("detected=%s translated=%s", det, out)
}

// TestTTSLiveGenerate hits the real Google TTS endpoint.
func TestTTSLiveGenerate(t *testing.T) {
	if os.Getenv("GOLDMD_LIVE") == "" {
		t.Skip("set GOLDMD_LIVE=1 to run live network test")
	}
	audio, err := ttsGenerate(context.Background(), "Hello this is a test of the voice", "en")
	if err != nil {
		t.Fatalf("ttsGenerate: %v", err)
	}
	if len(audio) < 1000 {
		t.Errorf("audio too small: %d", len(audio))
	}
	t.Logf("tts audio: %d bytes", len(audio))
}
