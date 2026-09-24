package main

import (
	"net/http"
	"strings"
	"testing"
)

// The default menu/alive picture is embedded, so it must always resolve even
// when the owner never set .botpic (the old CDN URL 404'd and an error page
// was uploaded as the "image").
func TestDefaultBotPicIsEmbedded(t *testing.T) {
	if len(defaultBotPicJPEG) < 1000 {
		t.Fatalf("embedded default jpeg too small: %d bytes", len(defaultBotPicJPEG))
	}
	if mime := imageMimeForBytes(defaultBotPicJPEG); mime != "image/jpeg" {
		t.Fatalf("embedded default mime = %q, want image/jpeg", mime)
	}
	data, err := fetchMenuImageURL(defaultBotPicMarker)
	if err != nil {
		t.Fatalf("fetchMenuImageURL(marker) error: %v", err)
	}
	if len(data) != len(defaultBotPicJPEG) {
		t.Fatalf("marker returned %d bytes, want embedded %d", len(data), len(defaultBotPicJPEG))
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		t.Fatalf("embedded default is not a detected image")
	}
}

// botPicURL must hand back the marker (not a dead URL) when nothing is set.
func TestBotPicURLFallsBackToMarker(t *testing.T) {
	s := &Session{}
	if got := s.botPicURL(); got != defaultBotPicMarker {
		t.Fatalf("botPicURL() = %q, want marker %q", got, defaultBotPicMarker)
	}
}

func TestImageMimeSniffing(t *testing.T) {
	if m := imageMimeForBytes([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}); m != "image/jpeg" {
		t.Fatalf("jpeg sniff = %q", m)
	}
	if m := imageMimeForBytes([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}); m != "image/png" {
		t.Fatalf("png sniff = %q", m)
	}
	if m := imageMimeForBytes([]byte("RIFF\x00\x00\x00\x00WEBP")); m != "image/webp" {
		t.Fatalf("webp sniff = %q", m)
	}
}
