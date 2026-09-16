package goldcmds

// ============================================================================
// GOLD-MD — .TRT  (GOOGLE TRANSLATOR)
// File: trt.go
// ============================================================================
// COMMAND:
//   .trt <lang> <text>          -> translate the given text into <lang>
//   .trt <lang>                 -> (reply to a message) translate it
//   .trt <text>                 -> auto target = ENGLISH
//
// Powered by Google Translate (free web endpoint, no API key):
//   https://clients5.google.com/translate_a/t?client=dict-chrome-ex
//   Response: [["translated text","detected_source_lang"]]
//
// Supports 130+ languages (full Google NMT list). The guidance message lists
// every supported language so the user can pick one easily.
//
// Aliases (Hidden): translate, tr, gtrans
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// trtLang is one supported language (code + display name).
type trtLang struct {
	Code string
	Name string
}

// trtLangs is the full Google Translate NMT language list (code -> name).
// Order is alphabetical by name for a clean guidance message.
var trtLangs = []trtLang{
	{"af", "AFRIKAANS"}, {"sq", "ALBANIAN"}, {"am", "AMHARIC"}, {"ar", "ARABIC"},
	{"hy", "ARMENIAN"}, {"as", "ASSAMESE"}, {"ay", "AYMARA"}, {"az", "AZERBAIJANI"},
	{"bm", "BAMBARA"}, {"eu", "BASQUE"}, {"be", "BELARUSIAN"}, {"bn", "BENGALI"},
	{"bho", "BHOJPURI"}, {"bs", "BOSNIAN"}, {"bg", "BULGARIAN"}, {"ca", "CATALAN"},
	{"ceb", "CEBUANO"}, {"ny", "CHICHEWA"}, {"zh-CN", "CHINESE (SIMPLIFIED)"},
	{"zh-TW", "CHINESE (TRADITIONAL)"}, {"co", "CORSICAN"}, {"hr", "CROATIAN"},
	{"cs", "CZECH"}, {"da", "DANISH"}, {"dv", "DIVEHI"}, {"nl", "DUTCH"},
	{"en", "ENGLISH"}, {"eo", "ESPERANTO"}, {"et", "ESTONIAN"}, {"ee", "EWE"},
	{"fil", "FILIPINO"}, {"fi", "FINNISH"}, {"fr", "FRENCH"}, {"fy", "FRISIAN"},
	{"gl", "GALICIAN"}, {"ka", "GEORGIAN"}, {"de", "GERMAN"}, {"el", "GREEK"},
	{"gn", "GUARANI"}, {"gu", "GUJARATI"}, {"ht", "HAITIAN CREOLE"}, {"ha", "HAUSA"},
	{"haw", "HAWAIIAN"}, {"he", "HEBREW"}, {"hi", "HINDI"}, {"hmn", "HMONG"},
	{"hu", "HUNGARIAN"}, {"is", "ICELANDIC"}, {"ig", "IGBO"}, {"ilo", "ILOKO"},
	{"id", "INDONESIAN"}, {"ga", "IRISH"}, {"it", "ITALIAN"}, {"ja", "JAPANESE"},
	{"jv", "JAVANESE"}, {"kn", "KANNADA"}, {"kk", "KAZAKH"}, {"km", "KHMER"},
	{"rw", "KINYARWANDA"}, {"gom", "KONKANI"}, {"ko", "KOREAN"}, {"kri", "KRIO"},
	{"ku", "KURDISH (KURMANJI)"}, {"ckb", "KURDISH (SORANI)"}, {"ky", "KYRGYZ"},
	{"lo", "LAO"}, {"la", "LATIN"}, {"lv", "LATVIAN"}, {"ln", "LINGALA"},
	{"lt", "LITHUANIAN"}, {"lg", "LUGANDA"}, {"lb", "LUXEMBOURGISH"},
	{"mk", "MACEDONIAN"}, {"mai", "MAITHILI"}, {"mg", "MALAGASY"}, {"ms", "MALAY"},
	{"ml", "MALAYALAM"}, {"mt", "MALTESE"}, {"mi", "MAORI"}, {"mr", "MARATHI"},
	{"mni-Mtei", "MEITEILON (MANIPURI)"}, {"lus", "MIZO"}, {"mn", "MONGOLIAN"},
	{"my", "MYANMAR (BURMESE)"}, {"ne", "NEPALI"}, {"no", "NORWEGIAN"},
	{"or", "ODIA (ORIYA)"}, {"om", "OROMO"}, {"ps", "PASHTO"}, {"fa", "PERSIAN"},
	{"pl", "POLISH"}, {"pt", "PORTUGUESE"}, {"pa", "PUNJABI"}, {"qu", "QUECHUA"},
	{"ro", "ROMANIAN"}, {"ru", "RUSSIAN"}, {"sm", "SAMOAN"}, {"sa", "SANSKRIT"},
	{"gd", "SCOTS GAELIC"}, {"sr", "SERBIAN"}, {"st", "SESOTHO"}, {"sn", "SHONA"},
	{"sd", "SINDHI"}, {"si", "SINHALA"}, {"sk", "SLOVAK"}, {"sl", "SLOVENIAN"},
	{"so", "SOMALI"}, {"es", "SPANISH"}, {"su", "SUNDANESE"}, {"sw", "SWAHILI"},
	{"sv", "SWEDISH"}, {"tg", "TAJIK"}, {"ta", "TAMIL"}, {"tt", "TATAR"},
	{"te", "TELUGU"}, {"th", "THAI"}, {"ti", "TIGRINYA"}, {"ts", "TSONGA"},
	{"tr", "TURKISH"}, {"tk", "TURKMEN"}, {"uk", "UKRAINIAN"}, {"ur", "URDU"},
	{"ug", "UYGHUR"}, {"uz", "UZBEK"}, {"vi", "VIETNAMESE"}, {"cy", "WELSH"},
	{"xh", "XHOSA"}, {"yi", "YIDDISH"}, {"yo", "YORUBA"}, {"zu", "ZULU"},
}

// trtLangIndex maps a lower-cased code OR name to the canonical code.
var trtLangIndex = func() map[string]string {
	m := make(map[string]string, len(trtLangs)*2)
	for _, l := range trtLangs {
		m[strings.ToLower(l.Code)] = l.Code
		m[strings.ToLower(l.Name)] = l.Code
	}
	// common aliases
	m["chinese"] = "zh-CN"
	m["mandarin"] = "zh-CN"
	m["tagalog"] = "fil"
	m["farsi"] = "fa"
	m["punjabi (pakistan)"] = "pa"
	m["roman urdu"] = "ur"
	return m
}()

// trtResolveLang turns a user token (code or name) into a canonical code.
// Returns ("", false) when the token is not a known language.
func trtResolveLang(tok string) (string, bool) {
	code, ok := trtLangIndex[strings.ToLower(strings.TrimSpace(tok))]
	return code, ok
}

// trtLangName returns the display name for a code (upper-case), or the code.
func trtLangName(code string) string {
	for _, l := range trtLangs {
		if strings.EqualFold(l.Code, code) {
			return l.Name
		}
	}
	return strings.ToUpper(code)
}

// trtGuide builds the guidance message listing every supported language.
func trtGuide(prefix string) string {
	var b strings.Builder
	b.WriteString("*🔰 TRANSLATOR 🔰*\n\n")
	b.WriteString("*TRANSLATE ANY TEXT INTO 130+ LANGUAGES INSTANTLY*\n\n")
	b.WriteString("*HOW TO USE:*\n")
	b.WriteString("*❮ " + prefix + "TRT <LANG> <TEXT> ❯*\n")
	b.WriteString("*EXAMPLE ❮ " + prefix + "TRT UR HELLO BROTHER ❯*\n\n")
	b.WriteString("*OR REPLY TO ANY MESSAGE AND TYPE:*\n")
	b.WriteString("*❮ " + prefix + "TRT <LANG> ❯*\n\n")
	b.WriteString("*IF YOU SKIP THE LANGUAGE, IT TRANSLATES TO ENGLISH*\n\n")
	b.WriteString("*SUPPORTED LANGUAGES:*\n")
	for _, l := range trtLangs {
		b.WriteString("*" + l.Code + " — " + l.Name + "*\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// trtTranslate calls the Google Translate free endpoint and returns the
// translated text plus the detected source language.
func trtTranslate(ctx context.Context, text, target string) (string, string, error) {
	u := "https://clients5.google.com/translate_a/t?client=dict-chrome-ex" +
		"&sl=auto&tl=" + url.QueryEscape(target) + "&q=" + url.QueryEscape(text)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("translate http %d", resp.StatusCode)
	}

	// Response shape: [["translated text","detected_lang"]]
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) == 0 {
		return "", "", fmt.Errorf("translate parse error")
	}
	var pair []string
	if err := json.Unmarshal(raw[0], &pair); err != nil || len(pair) == 0 {
		return "", "", fmt.Errorf("translate parse error")
	}
	translated := pair[0]
	detected := ""
	if len(pair) > 1 {
		detected = pair[1]
	}
	if strings.TrimSpace(translated) == "" {
		return "", "", fmt.Errorf("empty translation")
	}
	return translated, detected, nil
}

// trtSplitArgs separates an optional leading language token from the text.
// Returns (langCode, text, hadLang).
func trtSplitArgs(args []string) (string, string, bool) {
	if len(args) == 0 {
		return "", "", false
	}
	if code, ok := trtResolveLang(args[0]); ok {
		return code, strings.Join(args[1:], " "), true
	}
	return "", strings.Join(args, " "), false
}

func handleTRT(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		handleTRTAsync(ctx, s, info, args, prefix)
	})
}

func handleTRTAsync(ctx context.Context, s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	lang, text, hadLang := trtSplitArgs(args)

	// No inline text -> try the quoted/replied-to message.
	if strings.TrimSpace(text) == "" {
		if q := strings.TrimSpace(s.GetQuotedMessageText(info)); q != "" {
			text = q
		}
	}

	// Still nothing -> show guidance.
	if strings.TrimSpace(text) == "" {
		s.Reply(info, trtGuide(prefix))
		return
	}

	// Default target = English when the user did not name a language.
	if !hadLang || lang == "" {
		lang = "en"
	}

	waitID := s.ReplyWithID(info, "*TRANSLATING....*")
	defer func() { _ = s.DeleteMessage(info, waitID) }()

	translated, detected, err := trtTranslate(ctx, text, lang)
	if err != nil {
		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 TRANSLATION FAILED, PLEASE TRY AGAIN*")
		}
		return
	}

	var b strings.Builder
	b.WriteString("*🔰 TRANSLATOR 🔰*\n\n")
	if detected != "" {
		b.WriteString("*FROM ❯ " + trtLangName(detected) + "*\n")
	}
	b.WriteString("*TO ❯ " + trtLangName(lang) + "*\n\n")
	b.WriteString("*ORIGINAL:*\n" + text + "\n\n")
	b.WriteString("*TRANSLATION:*\n" + translated)
	s.Reply(info, b.String())
}

func init() {
	Register(Command{Name: "trt", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO TRANSLATE ANY TEXT INTO 130+ LANGUAGES USING GOOGLE TRANSLATE. USE IT AS .TRT <LANG> <TEXT> OR REPLY TO A MESSAGE WITH .TRT <LANG>.", Run: handleTRT})

	// aliases (Hidden)
	Register(Command{Name: "translate", Hidden: true, Run: handleTRT})
	Register(Command{Name: "tr", Hidden: true, Run: handleTRT})
	Register(Command{Name: "gtrans", Hidden: true, Run: handleTRT})
}
