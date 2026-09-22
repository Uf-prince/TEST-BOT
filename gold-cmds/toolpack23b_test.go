package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestToolpack23bGuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		"passwordstrength": passwordstrengthGuide("."),
		"randompassword":   randompasswordGuide("."),
		"randomuuid":       randomuuidGuide("."),
		"randint":          randintGuide("."),
		"randomdice":       randomdiceGuide("."),
		"randomcard":       randomcardGuide("."),
		"randomcoin":       randomcoinGuide("."),
		"randomwheel":      randomwheelGuide("."),
		"randomteam":       randomteamGuide("."),
		"randompick":       randompickGuide("."),
	}
	for name, g := range guides {
		if strings.Contains(g, `\n`) {
			t.Fatalf("%s guide contains literal backslash-n", name)
		}
		if !strings.Contains(g, "\n") {
			t.Fatalf("%s guide has no real newline", name)
		}
		if !strings.Contains(g, "🔰") {
			t.Fatalf("%s guide missing 🔰", name)
		}
	}
}

func TestToolpack23bLive(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		{"passwordstrength", handlePasswordstrength, []string{"Abc@12345"}},
		{"randompassword", handleRandompassword, []string{"16"}},
		{"randomuuid", handleRandomuuid, []string{}},
		{"randint", handleRandint, []string{"1", "100"}},
		{"randomdice", handleRandomdice, []string{"6", "2"}},
		{"randomcard", handleRandomcard, []string{}},
		{"randomcoin", handleRandomcoin, []string{}},
		{"randomwheel", handleRandomwheel, []string{"pizza,", "burger,", "pasta"}},
		{"randomteam", handleRandomteam, []string{"2", "ali,", "bob,", "sara,", "john"}},
		{"randompick", handleRandompick, []string{"red,", "green,", "blue"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "FAILED") || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FOUND, PLEASE TRY") {
			status = "FAIL"
		}
		t.Logf("%-16s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
