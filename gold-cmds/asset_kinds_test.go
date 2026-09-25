package goldcmds

import (
	"reflect"
	"testing"
)

// Every kind saved under one name must be delivered — a photo, video, sticker,
// circle and text can all share the name "umar" and none may shadow another.
func TestAssetKindsToSendReturnsAllSavedKinds(t *testing.T) {
	saved := map[string]bool{"img": true, "video": true, "sticker": true, "circle": true, "text": true}
	got := AssetKindsToSend("umar", func(kind string) bool { return saved[kind] })
	want := []string{"img", "video", "sticker", "circle", "text"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssetKindsToSend = %v, want %v", got, want)
	}
}

// A name saved as only a sticker must send only the sticker — the regression
// where an image of the same name was sent instead.
func TestAssetKindsToSendStickerOnly(t *testing.T) {
	saved := map[string]bool{"sticker": true}
	got := AssetKindsToSend("umar", func(kind string) bool { return saved[kind] })
	if !reflect.DeepEqual(got, []string{"sticker"}) {
		t.Fatalf("AssetKindsToSend = %v, want [sticker]", got)
	}
}

// Nothing saved => nothing to send.
func TestAssetKindsToSendEmpty(t *testing.T) {
	if got := AssetKindsToSend("nope", func(string) bool { return false }); len(got) != 0 {
		t.Fatalf("AssetKindsToSend = %v, want empty", got)
	}
}
