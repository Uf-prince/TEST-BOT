package goldcmds

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// TestToolpack45GuidesRenderRealNewlines ensures every new guide uses real
// newlines (no literal backslash-n) and carries the 🔰 marker.
func TestToolpack45GuidesRenderRealNewlines(t *testing.T) {
	guides := map[string]string{
		// pack 4
		"zip":         zipGuide("."),
		"holiday":     holidayGuide("."),
		"university":  universityGuide("."),
		"iss":         issGuide("."),
		"earthquake":  earthquakeGuide("."),
		"gender":      genderGuide("."),
		"age":         ageGuide("."),
		"nationality": nationalityGuide("."),
		"iban":        ibanGuide("."),
		"recipe":      recipeGuide("."),
		"cocktail":    cocktailGuide("."),
		"hijri":       hijriGuide("."),
		"qibla":       qiblaGuide("."),
		"advice":      adviceGuide("."),
		"trivia":      triviaGuide("."),
		// pack 5
		"myip":          myipGuide("."),
		"pincode":       pincodeGuide("."),
		"hackernews":    hackernewsGuide("."),
		"synonym":       synonymGuide("."),
		"mac":           macGuide("."),
		"sunrise":       sunriseGuide("."),
		"apod":          apodGuide("."),
		"starwars":      starwarsGuide("."),
		"rickandmorty":  rickmortyGuide("."),
		"chucknorris":   chucknorrisGuide("."),
		"kanye":         kanyeGuide("."),
		"randomword":    randomwordGuide("."),
		"catfact":       catfactGuide("."),
		"drug":          drugGuide("."),
		"stackoverflow": stackoverflowGuide("."),
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

// TestToolpack45Live exercises each new command against its live API.
func TestToolpack45Live(t *testing.T) {
	cases := []struct {
		name string
		fn   func(SessionBridge, types.MessageInfo, []string, string)
		args []string
	}{
		// pack 4
		{"zip", handleZip, []string{"us", "90210"}},
		{"holiday", handleHoliday, []string{"us"}},
		{"university", handleUniversity, []string{"harvard"}},
		{"iss", handleISS, nil},
		{"earthquake", handleEarthquake, nil},
		{"gender", handleGender, []string{"michael"}},
		{"age", handleAge, []string{"michael"}},
		{"nationality", handleNationality, []string{"michael"}},
		{"iban", handleIBAN, []string{"DE89370400440532013000"}},
		{"recipe", handleRecipe, []string{"pasta"}},
		{"cocktail", handleCocktail, []string{"margarita"}},
		{"hijri", handleHijri, nil},
		{"qibla", handleQibla, []string{"24.86", "67.01"}},
		{"advice", handleAdvice, nil},
		{"trivia", handleTrivia, nil},
		// pack 5
		{"myip", handleMyIP, nil},
		{"pincode", handlePincode, []string{"110001"}},
		{"hackernews", handleHackerNews, nil},
		{"synonym", handleSynonym, []string{"happy"}},
		{"mac", handleMac, []string{"44:38:39:ff:ef:57"}},
		{"sunrise", handleSunrise, []string{"24.86", "67.01"}},
		{"apod", handleApod, nil},
		{"starwars", handleStarwars, []string{"luke"}},
		{"rickandmorty", handleRickMorty, []string{"rick"}},
		{"chucknorris", handleChuckNorris, nil},
		{"kanye", handleKanye, nil},
		{"randomword", handleRandomWord, []string{"5"}},
		{"catfact", handleCatFact, nil},
		{"drug", handleDrug, []string{"ibuprofen"}},
		{"stackoverflow", handleStackOverflow, []string{"golang", "goroutine"}},
	}
	for _, c := range cases {
		b := &lgBridge{}
		c.fn(b, types.MessageInfo{}, c.args, ".")
		r := b.lastReply()
		status := "OK"
		if r == "" || strings.Contains(r, "NOT FOUND") || strings.Contains(r, "FAILED") ||
			strings.Contains(r, "UNAVAILABLE") || strings.Contains(r, "COULD NOT") ||
			strings.Contains(r, "NO RESULTS") || strings.Contains(r, "NO SYNONYMS") {
			status = "FAIL"
		}
		t.Logf("%-14s -> %s | %.90s", c.name, status, strings.ReplaceAll(r, "\n", " "))
		if status == "FAIL" {
			t.Errorf("%s failed: %q", c.name, r)
		}
		time.Sleep(400 * time.Millisecond)
	}
}
