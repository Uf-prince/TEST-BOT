package goldcmds

import (
	"strings"
	"testing"
)

// The visible command must live in TOOLS and follow the GOLD-MD desc style.
func TestSimDataVisibleRegistration(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	c, ok := byName["simdata"]
	if !ok {
		t.Fatal("command \"simdata\" not registered")
	}
	if c.Hidden {
		t.Error("simdata must be visible")
	}
	if c.Category != "TOOLS" {
		t.Errorf("simdata category = %q, want TOOLS", c.Category)
	}
	if !strings.HasPrefix(c.Desc, "THIS COMMAND IS USED TO") {
		t.Errorf("simdata desc not in GOLD-MD style: %q", c.Desc)
	}
	if c.Run == nil {
		t.Error("simdata has nil Run")
	}
}

// Every alias must exist, work, and stay hidden (never in the menu/guide).
func TestSimDataAliasesHidden(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, alias := range []string{"siminfo", "simdetails", "detailsim", "simdetail", "datasim"} {
		c, ok := byName[alias]
		if !ok {
			t.Errorf("alias %q not registered", alias)
			continue
		}
		if !c.Hidden {
			t.Errorf("alias %q must be hidden", alias)
		}
		if c.Run == nil {
			t.Errorf("alias %q has nil Run", alias)
		}
	}
}

// The guide must carry the mandatory "Pakistani SIMs only" note at the end.
func TestSimDataGuideNote(t *testing.T) {
	g := simDataGuide(".")
	if !strings.Contains(g, "YEH COMMAND SIRF PAKISTANI SIMS KA DATA DETA HAI") {
		t.Errorf("guide missing Hinglish note: %q", g)
	}
	if !strings.Contains(g, "2024 SE PEHLE KI JITNE BHI SIM NUMBERS HOGE UNKA DATA NAHI MILE GA OK ERROR AYE GA") {
		t.Errorf("guide missing pre-2024 note: %q", g)
	}
	if !strings.Contains(g, "THIS COMMAND ONLY GIVES DATA OF PAKISTANI SIMS NUMBERS") {
		t.Errorf("guide missing English note: %q", g)
	}
	if !strings.Contains(g, ".SIMDATA <NUMBER>") {
		t.Errorf("guide missing usage line: %q", g)
	}
	for _, alias := range []string{"SIMINFO", "SIMDETAILS", "DETAILSIM", "DATASIM"} {
		if strings.Contains(strings.ToUpper(g), alias) {
			t.Errorf("guide must not advertise alias %q", alias)
		}
	}
}

func TestSimDataCleanNumber(t *testing.T) {
	cases := map[string]string{
		"03122212427":     "03122212427",
		"+923122212427":   "923122212427",
		"0312-222-12427":  "031222212427",
		" 0312 222 12427": "031222212427",
	}
	for in, want := range cases {
		if got := simDataCleanNumber(in); got != want {
			t.Errorf("simDataCleanNumber(%q) = %q, want %q", in, got, want)
		}
	}
}
