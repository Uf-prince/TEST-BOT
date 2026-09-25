package main

import (
	"testing"

	goldcmds "gold-md/gold-cmds"
)

// End-to-end: save the SAME name under every asset kind and confirm each one
// still reads back with its own bytes + mime. This is the "ek doosre ki jagah
// le raha hai" regression — kinds must never overwrite one another.
func TestAllKindsCoexistUnderOneName(t *testing.T) {
	b := newAssetTestBridge(t)
	payloads := map[string]struct {
		data []byte
		mime string
	}{
		"img":     {[]byte("jpeg-bytes"), "image/jpeg"},
		"video":   {[]byte("mp4-bytes"), "video/mp4"},
		"sticker": {[]byte("webp-bytes"), "image/webp"},
		"circle":  {[]byte("circle-bytes"), "video/mp4"},
		"text":    {[]byte("Umar"), "text/plain"},
	}
	for kind, p := range payloads {
		if !b.SaveCustomAsset(kind, "umar", p.data, p.mime) {
			t.Fatalf("save %s failed", kind)
		}
	}
	for kind, p := range payloads {
		got, mime, ok := b.GetCustomAsset(kind, "umar")
		if !ok {
			t.Fatalf("%s missing after saving all kinds", kind)
		}
		if string(got) != string(p.data) || mime != p.mime {
			t.Errorf("%s = (%q,%q), want (%q,%q)", kind, got, mime, p.data, p.mime)
		}
	}
}

// The trigger must report EVERY saved kind for a shared name, so a name saved
// as an image AND a sticker delivers both instead of one shadowing the other.
func TestTriggerSendsEverySavedKind(t *testing.T) {
	b := newAssetTestBridge(t)
	for kind, mime := range map[string]string{
		"img": "image/jpeg", "video": "video/mp4", "sticker": "image/webp", "text": "text/plain",
	} {
		if !b.SaveCustomAsset(kind, "umar", []byte(kind+"-bytes"), mime) {
			t.Fatalf("save %s failed", kind)
		}
	}
	got := goldcmds.AssetKindsToSend("umar", func(kind string) bool {
		data, _, _, found := b.GetCustomAssetMeta(kind, "umar")
		return found && len(data) > 0
	})
	if len(got) != 4 {
		t.Fatalf("AssetKindsToSend = %v, want all 4 saved kinds", got)
	}
	for i, want := range []string{"img", "video", "sticker", "text"} {
		if got[i] != want {
			t.Errorf("kinds[%d] = %q, want %q", i, got[i], want)
		}
	}
}

// Deleting one kind must leave the others intact (the "ek ko hata le to doosra
// apni jagah aa jata hai" symptom came from only ever storing/serving one kind).
func TestDeletingOneKindKeepsOthers(t *testing.T) {
	b := newAssetTestBridge(t)
	for kind, mime := range map[string]string{
		"img": "image/jpeg", "video": "video/mp4", "sticker": "image/webp",
	} {
		if !b.SaveCustomAsset(kind, "umar", []byte(kind+"-bytes"), mime) {
			t.Fatalf("save %s failed", kind)
		}
	}
	if !b.DeleteCustomAsset("img", "umar") {
		t.Fatal("delete img failed")
	}
	if _, _, ok := b.GetCustomAsset("img", "umar"); ok {
		t.Error("img still present after delete")
	}
	for _, kind := range []string{"video", "sticker"} {
		if _, _, ok := b.GetCustomAsset(kind, "umar"); !ok {
			t.Errorf("%s vanished when img was deleted", kind)
		}
	}
}
