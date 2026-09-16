package goldcmds

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// lidBridge is a fake SessionBridge that records ResolveToPN calls and
// simulates a LID→PN mapping so we can prove the conversion is applied.
type lidBridge struct {
	SessionBridge
	replies     []string
	resolved    []types.JID
	lidToPN     map[string]string
	mentioned   []string
	quotedID    string
	quotedSndr  string
	hasQuoted   bool
	ownerResult bool
}

func (f *lidBridge) Reply(info types.MessageInfo, text string) {
	f.replies = append(f.replies, text)
}

func (f *lidBridge) IsOwner(info types.MessageInfo) bool { return f.ownerResult }

func (f *lidBridge) GetMentionedJIDs(info types.MessageInfo) ([]string, bool) {
	if len(f.mentioned) == 0 {
		return nil, false
	}
	return f.mentioned, true
}

func (f *lidBridge) GetQuotedMessageID(info types.MessageInfo) (string, string, bool) {
	if !f.hasQuoted {
		return "", "", false
	}
	return f.quotedID, f.quotedSndr, true
}

// ResolveToPN simulates the real LID→PN conversion.
func (f *lidBridge) ResolveToPN(jid types.JID) types.JID {
	f.resolved = append(f.resolved, jid)
	if jid.Server == types.HiddenUserServer {
		if pn, ok := f.lidToPN[jid.User]; ok {
			return types.NewJID(pn, types.DefaultUserServer)
		}
	}
	return jid
}

// TestGroupMoreCommandsRegistered verifies every new command name is present in
// the registry (visible or hidden alias).
func TestGroupMoreCommandsRegistered(t *testing.T) {
	want := []string{
		"gpp", "gppremove", "gdeldesc", "gowner", "gcreated",
		"gsettings", "gphoto", "glist", "ginfo", "gjoin", "gleave",
		"gpromote", "gremove", "gadd", "grole",
		"gcount", "gsearch", "gstats", "grank", "gtop", "gactive",
		"gcheck", "gblock", "gunblock", "gblocklist", "gpic", "gabout",
		"gpin", "gunpin", "garchive", "gunarchive", "gmarkread",
		"gtyping", "gstoptyping", "hidetag", "gpromoteall",
		"gdemoteall", "gkickall", "gclean", "gsummary", "gid", "gname2",
		"gapproval", "gaddmode", "gnewlink", "ggetlink",
		"gsetdesc", "gmembers", "gadmins", "grequests",
		"gapprove", "greject", "gversion", "ghelp",
		// hidden aliases that must still work
		"gmention", "gtag", "gsetname", "join", "pmt", "dmt",
	}
	have := map[string]bool{}
	for _, c := range Commands() {
		have[c.Name] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("command %q is not registered", w)
		}
	}
}

// TestCollectTargetsLIDConversion proves collectTargets converts LID mentions
// to their real PN JIDs (MANDATORY LID→PN conversion).
func TestCollectTargetsLIDConversion(t *testing.T) {
	fb := &lidBridge{
		lidToPN:   map[string]string{"111222": "923001112222"},
		mentioned: []string{"111222@lid"},
	}
	var info types.MessageInfo
	out := collectTargets(fb, info, nil)
	if len(out) != 1 {
		t.Fatalf("expected 1 target, got %d (%v)", len(out), out)
	}
	if out[0].Server != types.DefaultUserServer || out[0].User != "923001112222" {
		t.Fatalf("expected PN 923001112222@s.whatsapp.net, got %s", out[0].String())
	}
	if len(fb.resolved) == 0 {
		t.Fatalf("ResolveToPN was never called — LID→PN conversion missing")
	}
}

// TestTargetJIDsLIDConversion proves the OLD group.go targetJIDs helper also
// converts LID mentions to PN.
func TestTargetJIDsLIDConversion(t *testing.T) {
	fb := &lidBridge{
		lidToPN:   map[string]string{"555666": "923005556666"},
		mentioned: []string{"555666@lid"},
	}
	var info types.MessageInfo
	out := targetJIDs(fb, info)
	if len(out) != 1 {
		t.Fatalf("expected 1 target, got %d (%v)", len(out), out)
	}
	if out[0].Server != types.DefaultUserServer || out[0].User != "923005556666" {
		t.Fatalf("expected PN 923005556666@s.whatsapp.net, got %s", out[0].String())
	}
}

// TestTargetJIDsQuotedLIDConversion proves the replied-to fallback also converts.
func TestTargetJIDsQuotedLIDConversion(t *testing.T) {
	fb := &lidBridge{
		lidToPN:    map[string]string{"777888": "923007778888"},
		hasQuoted:  true,
		quotedID:   "ABC123",
		quotedSndr: "777888@lid",
	}
	var info types.MessageInfo
	out := targetJIDs(fb, info)
	if len(out) != 1 {
		t.Fatalf("expected 1 target, got %d (%v)", len(out), out)
	}
	if out[0].Server != types.DefaultUserServer || out[0].User != "923007778888" {
		t.Fatalf("expected PN 923007778888@s.whatsapp.net, got %s", out[0].String())
	}
}

// TestExtractInviteCode covers the invite-link parser used by ginfo/gjoin.
func TestExtractInviteCode(t *testing.T) {
	cases := map[string]string{
		"https://chat.whatsapp.com/AbCdEf123":       "AbCdEf123",
		"https://chat.whatsapp.com/AbCdEf123?mode=x": "AbCdEf123",
		"AbCdEf123":                                  "AbCdEf123",
		"":                                           "",
	}
	for in, want := range cases {
		if got := extractInviteCode(in); got != want {
			t.Errorf("extractInviteCode(%q) = %q, want %q", in, got, want)
		}
	}
}
