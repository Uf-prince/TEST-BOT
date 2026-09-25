package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK (10 high-value everyday commands)
// File: toolpack.go
// ============================================================================
// A bundle of 10 genuinely useful, always-working TOOLS commands. Every one
// uses a FREE public API (no key) and matches the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS) so it blends into the bot seamlessly.
//
//   .prayer <city>              -> daily prayer (namaz) times + hijri date
//   .quran                      -> full guide (all 114 surahs)
//   .quran <surah>              -> full surah (Arabic + Urdu + English tarjuma)
//   .quran <surah> <ayah>       -> that specific ayah (Arabic + Urdu + English)
//   .dictionary <word>          -> English word meaning + example
//   .currency <amt> <from> <to> -> live currency conversion
//   .timezone <zone>            -> current date & time in any timezone
//   .news [category]            -> top news headlines
//   .lyrics <artist> - <song>   -> song lyrics
//   .github <username>          -> GitHub user profile
//   .anime <title>              -> anime info (rating, episodes, synopsis)
//   .pokemon <name>             -> Pokemon stats & info
//
// All commands run inside RunWithTimeout (which carries the per-session /
// per-user latest-wins guard), so a newer request instantly cancels an older
// one — same behaviour as every other GOLD-MD command.
// ============================================================================

import (
	"context"
	"encoding/xml"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// .PRAYER — DAILY PRAYER (NAMAZ) TIMES
// ============================================================================

func prayerGuide(prefix string) string {
	return "*🔰 PRAYER TIMES 🔰*\n\n" +
		"*GET TODAY'S NAMAZ TIMES OF ANY CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PRAYER <CITY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PRAYER LAHORE ❯*"
}

func handlePrayer(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		city := strings.TrimSpace(strings.Join(args, " "))
		if city == "" {
			s.Reply(info, prayerGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PRAYER TIMES....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.aladhan.com/v1/timingsByCity?city=" + url.QueryEscape(city) + "&country=&method=2"
		var res struct {
			Code int `json:"code"`
			Data struct {
				Timings struct {
					Fajr    string `json:"Fajr"`
					Sunrise string `json:"Sunrise"`
					Dhuhr   string `json:"Dhuhr"`
					Asr     string `json:"Asr"`
					Maghrib string `json:"Maghrib"`
					Isha    string `json:"Isha"`
				} `json:"timings"`
				Date struct {
					Readable string `json:"readable"`
					Hijri    struct {
						Date string `json:"date"`
					} `json:"hijri"`
				} `json:"date"`
				Meta struct {
					Timezone string `json:"timezone"`
				} `json:"meta"`
			} `json:"data"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 CITY NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		t := res.Data.Timings
		var b strings.Builder
		b.WriteString("*🔰 PRAYER TIMES 🔰*\n\n")
		b.WriteString("*📍 CITY ❯ " + strings.ToUpper(city) + "*\n")
		b.WriteString("*📅 DATE ❯ " + res.Data.Date.Readable + "*\n")
		b.WriteString("*🌙 HIJRI ❯ " + res.Data.Date.Hijri.Date + "*\n")
		b.WriteString("*🕐 ZONE ❯ " + res.Data.Meta.Timezone + "*\n\n")
		b.WriteString("*🕌 FAJR ❯ " + t.Fajr + "*\n")
		b.WriteString("*🌅 SUNRISE ❯ " + t.Sunrise + "*\n")
		b.WriteString("*☀️ DHUHR ❯ " + t.Dhuhr + "*\n")
		b.WriteString("*🌤️ ASR ❯ " + t.Asr + "*\n")
		b.WriteString("*🌇 MAGHRIB ❯ " + t.Maghrib + "*\n")
		b.WriteString("*🌃 ISHA ❯ " + t.Isha + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .QURAN — HOLY QURAN (full surah / specific ayah)
// ============================================================================

// quranSurah describes one surah for the name -> number lookup.
type quranSurah struct {
	Number int
	Name   string
	Ayahs  int
}

// quranSurahs maps a normalized (lower-case, letters-only) surah name to its
// number + English name + ayah count. Covers all 114 surahs.
var quranSurahs = map[string]quranSurah{
	"alfaatiha":     {1, "Al-Faatiha", 7},
	"albaqara":      {2, "Al-Baqara", 286},
	"aaliimraan":    {3, "Aal-i-Imraan", 200},
	"annisaa":       {4, "An-Nisaa", 176},
	"almaaida":      {5, "Al-Maaida", 120},
	"alanaam":       {6, "Al-An'aam", 165},
	"alaraaf":       {7, "Al-A'raaf", 206},
	"alanfaal":      {8, "Al-Anfaal", 75},
	"attawba":       {9, "At-Tawba", 129},
	"yunus":         {10, "Yunus", 109},
	"hud":           {11, "Hud", 123},
	"yusuf":         {12, "Yusuf", 111},
	"arrad":         {13, "Ar-Ra'd", 43},
	"ibrahim":       {14, "Ibrahim", 52},
	"alhijr":        {15, "Al-Hijr", 99},
	"annahl":        {16, "An-Nahl", 128},
	"alisraa":       {17, "Al-Israa", 111},
	"alkahf":        {18, "Al-Kahf", 110},
	"maryam":        {19, "Maryam", 98},
	"taahaa":        {20, "Taa-Haa", 135},
	"alanbiyaa":     {21, "Al-Anbiyaa", 112},
	"alhajj":        {22, "Al-Hajj", 78},
	"almuminoon":    {23, "Al-Muminoon", 118},
	"annoor":        {24, "An-Noor", 64},
	"alfurqaan":     {25, "Al-Furqaan", 77},
	"ashshuaraa":    {26, "Ash-Shu'araa", 227},
	"annaml":        {27, "An-Naml", 93},
	"alqasas":       {28, "Al-Qasas", 88},
	"alankaboot":    {29, "Al-Ankaboot", 69},
	"arroom":        {30, "Ar-Room", 60},
	"luqman":        {31, "Luqman", 34},
	"assajda":       {32, "As-Sajda", 30},
	"alahzaab":      {33, "Al-Ahzaab", 73},
	"saba":          {34, "Saba", 54},
	"faatir":        {35, "Faatir", 45},
	"yaseen":        {36, "Yaseen", 83},
	"assaaffaat":    {37, "As-Saaffaat", 182},
	"saad":          {38, "Saad", 88},
	"azzumar":       {39, "Az-Zumar", 75},
	"ghafir":        {40, "Ghafir", 85},
	"fussilat":      {41, "Fussilat", 54},
	"ashshura":      {42, "Ash-Shura", 53},
	"azzukhruf":     {43, "Az-Zukhruf", 89},
	"addukhaan":     {44, "Ad-Dukhaan", 59},
	"aljaathiya":    {45, "Al-Jaathiya", 37},
	"alahqaf":       {46, "Al-Ahqaf", 35},
	"muhammad":      {47, "Muhammad", 38},
	"alfath":        {48, "Al-Fath", 29},
	"alhujuraat":    {49, "Al-Hujuraat", 18},
	"qaaf":          {50, "Qaaf", 45},
	"adhdhaariyat":  {51, "Adh-Dhaariyat", 60},
	"attur":         {52, "At-Tur", 49},
	"annajm":        {53, "An-Najm", 62},
	"alqamar":       {54, "Al-Qamar", 55},
	"arrahmaan":     {55, "Ar-Rahmaan", 78},
	"alwaaqia":      {56, "Al-Waaqia", 96},
	"alhadid":       {57, "Al-Hadid", 29},
	"almujaadila":   {58, "Al-Mujaadila", 22},
	"alhashr":       {59, "Al-Hashr", 24},
	"almumtahana":   {60, "Al-Mumtahana", 13},
	"assaff":        {61, "As-Saff", 14},
	"aljumua":       {62, "Al-Jumu'a", 11},
	"almunaafiqoon": {63, "Al-Munaafiqoon", 11},
	"attaghaabun":   {64, "At-Taghaabun", 18},
	"attalaaq":      {65, "At-Talaaq", 12},
	"attahrim":      {66, "At-Tahrim", 12},
	"almulk":        {67, "Al-Mulk", 30},
	"alqalam":       {68, "Al-Qalam", 52},
	"alhaaqqa":      {69, "Al-Haaqqa", 52},
	"almaaarij":     {70, "Al-Ma'aarij", 44},
	"nooh":          {71, "Nooh", 28},
	"aljinn":        {72, "Al-Jinn", 28},
	"almuzzammil":   {73, "Al-Muzzammil", 20},
	"almuddaththir": {74, "Al-Muddaththir", 56},
	"alqiyaama":     {75, "Al-Qiyaama", 40},
	"alinsaan":      {76, "Al-Insaan", 31},
	"almursalaat":   {77, "Al-Mursalaat", 50},
	"annaba":        {78, "An-Naba", 40},
	"annaaziaat":    {79, "An-Naazi'aat", 46},
	"abasa":         {80, "Abasa", 42},
	"attakwir":      {81, "At-Takwir", 29},
	"alinfitaar":    {82, "Al-Infitaar", 19},
	"almutaffifin":  {83, "Al-Mutaffifin", 36},
	"alinshiqaaq":   {84, "Al-Inshiqaaq", 25},
	"alburooj":      {85, "Al-Burooj", 22},
	"attaariq":      {86, "At-Taariq", 17},
	"alalaa":        {87, "Al-A'laa", 19},
	"alghaashiya":   {88, "Al-Ghaashiya", 26},
	"alfajr":        {89, "Al-Fajr", 30},
	"albalad":       {90, "Al-Balad", 20},
	"ashshams":      {91, "Ash-Shams", 15},
	"allail":        {92, "Al-Lail", 21},
	"addhuhaa":      {93, "Ad-Dhuhaa", 11},
	"ashsharh":      {94, "Ash-Sharh", 8},
	"attin":         {95, "At-Tin", 8},
	"alalaq":        {96, "Al-Alaq", 19},
	"alqadr":        {97, "Al-Qadr", 5},
	"albayyina":     {98, "Al-Bayyina", 8},
	"azzalzala":     {99, "Az-Zalzala", 8},
	"alaadiyaat":    {100, "Al-Aadiyaat", 11},
	"alqaaria":      {101, "Al-Qaari'a", 11},
	"attakaathur":   {102, "At-Takaathur", 8},
	"alasr":         {103, "Al-Asr", 3},
	"alhumaza":      {104, "Al-Humaza", 9},
	"alfil":         {105, "Al-Fil", 5},
	"quraish":       {106, "Quraish", 4},
	"almaaun":       {107, "Al-Maa'un", 7},
	"alkawthar":     {108, "Al-Kawthar", 3},
	"alkaafiroon":   {109, "Al-Kaafiroon", 6},
	"annasr":        {110, "An-Nasr", 3},
	"almasad":       {111, "Al-Masad", 5},
	"alikhlaas":     {112, "Al-Ikhlaas", 4},
	"alfalaq":       {113, "Al-Falaq", 5},
	"annaas":        {114, "An-Naas", 6}}

// quranSurahListText is the full 114-surah reference shown in the guide.
const quranSurahListText = "1. Al-Faatiha (7) | 2. Al-Baqara (286)\n3. Aal-i-Imraan (200) | 4. An-Nisaa (176)\n5. Al-Maaida (120) | 6. Al-An'aam (165)\n7. Al-A'raaf (206) | 8. Al-Anfaal (75)\n9. At-Tawba (129) | 10. Yunus (109)\n11. Hud (123) | 12. Yusuf (111)\n13. Ar-Ra'd (43) | 14. Ibrahim (52)\n15. Al-Hijr (99) | 16. An-Nahl (128)\n17. Al-Israa (111) | 18. Al-Kahf (110)\n19. Maryam (98) | 20. Taa-Haa (135)\n21. Al-Anbiyaa (112) | 22. Al-Hajj (78)\n23. Al-Muminoon (118) | 24. An-Noor (64)\n25. Al-Furqaan (77) | 26. Ash-Shu'araa (227)\n27. An-Naml (93) | 28. Al-Qasas (88)\n29. Al-Ankaboot (69) | 30. Ar-Room (60)\n31. Luqman (34) | 32. As-Sajda (30)\n33. Al-Ahzaab (73) | 34. Saba (54)\n35. Faatir (45) | 36. Yaseen (83)\n37. As-Saaffaat (182) | 38. Saad (88)\n39. Az-Zumar (75) | 40. Ghafir (85)\n41. Fussilat (54) | 42. Ash-Shura (53)\n43. Az-Zukhruf (89) | 44. Ad-Dukhaan (59)\n45. Al-Jaathiya (37) | 46. Al-Ahqaf (35)\n47. Muhammad (38) | 48. Al-Fath (29)\n49. Al-Hujuraat (18) | 50. Qaaf (45)\n51. Adh-Dhaariyat (60) | 52. At-Tur (49)\n53. An-Najm (62) | 54. Al-Qamar (55)\n55. Ar-Rahmaan (78) | 56. Al-Waaqia (96)\n57. Al-Hadid (29) | 58. Al-Mujaadila (22)\n59. Al-Hashr (24) | 60. Al-Mumtahana (13)\n61. As-Saff (14) | 62. Al-Jumu'a (11)\n63. Al-Munaafiqoon (11) | 64. At-Taghaabun (18)\n65. At-Talaaq (12) | 66. At-Tahrim (12)\n67. Al-Mulk (30) | 68. Al-Qalam (52)\n69. Al-Haaqqa (52) | 70. Al-Ma'aarij (44)\n71. Nooh (28) | 72. Al-Jinn (28)\n73. Al-Muzzammil (20) | 74. Al-Muddaththir (56)\n75. Al-Qiyaama (40) | 76. Al-Insaan (31)\n77. Al-Mursalaat (50) | 78. An-Naba (40)\n79. An-Naazi'aat (46) | 80. Abasa (42)\n81. At-Takwir (29) | 82. Al-Infitaar (19)\n83. Al-Mutaffifin (36) | 84. Al-Inshiqaaq (25)\n85. Al-Burooj (22) | 86. At-Taariq (17)\n87. Al-A'laa (19) | 88. Al-Ghaashiya (26)\n89. Al-Fajr (30) | 90. Al-Balad (20)\n91. Ash-Shams (15) | 92. Al-Lail (21)\n93. Ad-Dhuhaa (11) | 94. Ash-Sharh (8)\n95. At-Tin (8) | 96. Al-Alaq (19)\n97. Al-Qadr (5) | 98. Al-Bayyina (8)\n99. Az-Zalzala (8) | 100. Al-Aadiyaat (11)\n101. Al-Qaari'a (11) | 102. At-Takaathur (8)\n103. Al-Asr (3) | 104. Al-Humaza (9)\n105. Al-Fil (5) | 106. Quraish (4)\n107. Al-Maa'un (7) | 108. Al-Kawthar (3)\n109. Al-Kaafiroon (6) | 110. An-Nasr (3)\n111. Al-Masad (5) | 112. Al-Ikhlaas (4)\n113. Al-Falaq (5) | 114. An-Naas (6)"

func quranGuide(prefix string) string {
	return "*🔰 HOLY QURAN 🔰*\n\n" +
		"*READ THE HOLY QURAN WITH URDU + ENGLISH TARJUMA*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "QURAN <SURAH> ❯*\n" +
		"*FULL SURAH (ARABIC + URDU + ENGLISH)*\n" +
		"*EXAMPLE ❮ " + prefix + "QURAN YASEEN ❯*\n\n" +
		"*❮ " + prefix + "QURAN <SURAH> <AYAH> ❯*\n" +
		"*THAT SPECIFIC AYAH (ARABIC + URDU + ENGLISH)*\n" +
		"*EXAMPLE ❮ " + prefix + "QURAN YASEEN 5 ❯*\n\n" +
		"*📖 ALL 114 SURAHS (NAME + AYAH COUNT):*\n" +
		quranSurahListText
}

// quranNormalizeKey lower-cases a surah name and strips everything except
// a-z so "Al-Faatiha", "al faatiha" and "alfaatiha" all match.
func quranNormalizeKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// quranLookupSurah resolves a user-supplied surah name (or number) to a surah.
func quranLookupSurah(name string) (quranSurah, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return quranSurah{}, false
	}
	// numeric surah number
	if n, err := strconv.Atoi(name); err == nil {
		for _, s := range quranSurahs {
			if s.Number == n {
				return s, true
			}
		}
		return quranSurah{}, false
	}
	key := quranNormalizeKey(name)
	if s, ok := quranSurahs[key]; ok {
		return s, true
	}
	// prefix match (e.g. "yaseen" typed as "yasin")
	for k, s := range quranSurahs {
		if strings.HasPrefix(k, key) || strings.HasPrefix(key, k) {
			return s, true
		}
	}
	// substring match (e.g. "kahf" -> "alkahf", "ikhlas" -> "alikhlaas")
	if len(key) >= 3 {
		for k, s := range quranSurahs {
			if strings.Contains(k, key) || strings.Contains(key, k) {
				return s, true
			}
		}
	}
	// consonant-skeleton match for transliteration variants
	// (e.g. "yasin" and "yaseen" both reduce to "ysn")
	skel := quranSkeleton(key)
	if len(skel) >= 2 {
		for k, s := range quranSurahs {
			ks := quranSkeleton(k)
			if ks == skel || strings.Contains(ks, skel) || strings.Contains(skel, ks) {
				return s, true
			}
		}
	}
	return quranSurah{}, false
}

// quranSkeleton strips vowels so transliteration variants collapse together.
func quranSkeleton(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'a', 'e', 'i', 'o', 'u':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// quranAyah is one ayah across the three editions we fetch.
type quranAyah struct {
	Arabic  string
	Urdu    string
	English string
	Number  int
}

// quranFetchSurah fetches a full surah in Arabic + Urdu + English.
func quranFetchSurah(ctx context.Context, number int) (quranSurah, []quranAyah, bool) {
	u := "https://api.alquran.cloud/v1/surah/" + strconv.Itoa(number) + "/editions/quran-uthmani,ur.jalandhry,en.asad"
	var res struct {
		Code int `json:"code"`
		Data []struct {
			Number      int    `json:"number"`
			EnglishName string `json:"englishName"`
			Name        string `json:"name"`
			Ayahs       []struct {
				Number int    `json:"numberInSurah"`
				Text   string `json:"text"`
			} `json:"ayahs"`
		} `json:"data"`
	}
	if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 || len(res.Data) < 3 {
		return quranSurah{}, nil, false
	}
	ar := res.Data[0]
	ur := res.Data[1]
	en := res.Data[2]
	n := len(ar.Ayahs)
	if n == 0 {
		return quranSurah{}, nil, false
	}
	out := make([]quranAyah, 0, n)
	for i := 0; i < n; i++ {
		a := quranAyah{Number: ar.Ayahs[i].Number, Arabic: ar.Ayahs[i].Text}
		if i < len(ur.Ayahs) {
			a.Urdu = ur.Ayahs[i].Text
		}
		if i < len(en.Ayahs) {
			a.English = en.Ayahs[i].Text
		}
		out = append(out, a)
	}
	return quranSurah{Number: ar.Number, Name: ar.EnglishName, Ayahs: n}, out, true
}

// quranFetchAyah fetches a single ayah (surah:ayah) in Arabic + Urdu + English.
func quranFetchAyah(ctx context.Context, surah, ayah int) (quranAyah, bool) {
	u := "https://api.alquran.cloud/v1/ayah/" + strconv.Itoa(surah) + ":" + strconv.Itoa(ayah) + "/editions/quran-uthmani,ur.jalandhry,en.asad"
	var res struct {
		Code int `json:"code"`
		Data []struct {
			Number int    `json:"numberInSurah"`
			Text   string `json:"text"`
		} `json:"data"`
	}
	if err := funGetJSON(ctx, u, &res); err != nil || res.Code != 200 || len(res.Data) < 3 {
		return quranAyah{}, false
	}
	return quranAyah{
		Number:  res.Data[0].Number,
		Arabic:  res.Data[0].Text,
		Urdu:    res.Data[1].Text,
		English: res.Data[2].Text,
	}, true
}

func handleQuran(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		// ── .quran (no args) → full guide (all 114 surahs) ──
		if len(args) == 0 {
			s.Reply(info, quranGuide(prefix))
			return
		}

		// ── .quran <surah> [ayah] ──
		surahName := args[0]
		sr, ok := quranLookupSurah(surahName)
		if !ok {
			s.Reply(info, "*🔰 SURAH NOT FOUND, PLEASE CHECK THE NAME*\n\n"+quranGuide(prefix))
			return
		}

		// specific ayah
		if len(args) >= 2 {
			ayahNum, err := strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil || ayahNum < 1 || ayahNum > sr.Ayahs {
				s.Reply(info, "*🔰 INVALID AYAH NUMBER — "+sr.Name+" HAS "+strconv.Itoa(sr.Ayahs)+" AYAHS*")
				return
			}
			waitID := s.ReplyWithID(info, "*FETCHING AYAH....*")
			defer func() { _ = s.DeleteMessage(info, waitID) }()
			a, ok := quranFetchAyah(ctx, sr.Number, ayahNum)
			if !ok {
				if !ctxTimedOut(ctx) {
					funFail(s, info, "QURAN AYAH")
				}
				return
			}
			var b strings.Builder
			b.WriteString("*🔰 HOLY QURAN 🔰*\n\n")
			b.WriteString("*📖 SURAH ❯ " + strings.ToUpper(sr.Name) + "*\n")
			b.WriteString("*🔢 AYAH ❯ " + strconv.Itoa(a.Number) + " / " + strconv.Itoa(sr.Ayahs) + "*\n\n")
			b.WriteString(a.Arabic + "\n\n")
			b.WriteString("*🇵🇰 URDU:*\n" + a.Urdu + "\n\n")
			b.WriteString("*🇬🇧 ENGLISH:*\n" + a.English)
			s.Reply(info, b.String())
			return
		}

		// full surah
		waitID := s.ReplyWithID(info, "*FETCHING SURAH "+strings.ToUpper(sr.Name)+"....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		meta, ayahs, ok := quranFetchSurah(ctx, sr.Number)
		if !ok {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SURAH")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 HOLY QURAN 🔰*\n\n")
		b.WriteString("*📖 SURAH ❯ " + strings.ToUpper(meta.Name) + "*\n")
		b.WriteString("*🔢 AYAHS ❯ " + strconv.Itoa(meta.Ayahs) + "*\n\n")
		truncated := false
		for _, a := range ayahs {
			block := "*" + strconv.Itoa(a.Number) + ".* " + a.Arabic + "\n" +
				"*🇵🇰* " + a.Urdu + "\n" +
				"*🇬🇧* " + a.English + "\n\n"
			if b.Len()+len(block) > 12000 {
				truncated = true
				break
			}
			b.WriteString(block)
		}
		if truncated {
			b.WriteString("*...SURAH BAHUT LAMBI HAI — BAQI AYAHS KE LIYE " + prefix + "QURAN " + strings.ToUpper(meta.Name) + " <AYAH> USE KAREIN*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ============================================================================
// .DICTIONARY — ENGLISH WORD MEANING
// ============================================================================

func dictionaryGuide(prefix string) string {
	return "*🔰 DICTIONARY 🔰*\n\n" +
		"*GET THE MEANING OF ANY ENGLISH WORD*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "DICTIONARY <WORD> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "DICTIONARY HELLO ❯*"
}

// stripHTML removes HTML tags and decodes the few entities Wiktionary emits,
// so definitions read as clean plain text on WhatsApp.
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var htmlBlockRe = regexp.MustCompile(`(?is)<(style|script)[^>]*>.*?</(style|script)>`)

func stripHTML(s string) string {
	s = htmlBlockRe.ReplaceAllString(s, "")
	s = htmlTagRe.ReplaceAllString(s, "")
	amp := string(rune(38))
	s = strings.ReplaceAll(s, amp+"amp;", amp)
	s = strings.ReplaceAll(s, amp+"quot;", string(rune(34)))
	s = strings.ReplaceAll(s, amp+"#39;", string(rune(39)))
	s = strings.ReplaceAll(s, amp+"apos;", string(rune(39)))
	s = strings.ReplaceAll(s, amp+"lt;", string(rune(60)))
	s = strings.ReplaceAll(s, amp+"gt;", string(rune(62)))
	s = strings.ReplaceAll(s, amp+"nbsp;", " ")
	return strings.Join(strings.Fields(s), " ")
}

// dictWiktionary fetches English definitions from the Wiktionary REST API.
func dictWiktionary(ctx context.Context, word string) (string, []struct {
	POS  string
	Defs []string
}, bool) {
	u := "https://en.wiktionary.org/api/rest_v1/page/definition/" + url.PathEscape(word)
	var res map[string][]struct {
		PartOfSpeech string `json:"partOfSpeech"`
		Definitions  []struct {
			Definition string   `json:"definition"`
			Examples   []string `json:"examples"`
		} `json:"definitions"`
	}
	if err := funGetJSON(ctx, u, &res); err != nil {
		return "", nil, false
	}
	entries, ok := res["en"]
	if !ok || len(entries) == 0 {
		return "", nil, false
	}
	out := make([]struct {
		POS  string
		Defs []string
	}, 0, len(entries))
	for _, e := range entries {
		pos := strings.TrimSpace(e.PartOfSpeech)
		if pos == "" {
			continue
		}
		defs := make([]string, 0, 2)
		for _, d := range e.Definitions {
			txt := stripHTML(d.Definition)
			if txt == "" {
				continue
			}
			defs = append(defs, txt)
			if len(defs) >= 2 {
				break
			}
		}
		if len(defs) == 0 {
			continue
		}
		out = append(out, struct {
			POS  string
			Defs []string
		}{pos, defs})
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 {
		return "", nil, false
	}
	return word, out, true
}

// dictDatamuse is a fallback source (definitions only, no POS grouping).
func dictDatamuse(ctx context.Context, word string) ([]string, bool) {
	u := "https://api.datamuse.com/words?sp=" + url.QueryEscape(word) + "&md=d&max=1"
	var res []struct {
		Word string   `json:"word"`
		Defs []string `json:"defs"`
	}
	if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 || len(res[0].Defs) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(res[0].Defs))
	for _, d := range res[0].Defs {
		// datamuse defs look like "n\tdefinition text"
		if i := strings.IndexByte(d, '\t'); i >= 0 {
			d = d[i+1:]
		}
		d = stripHTML(strings.TrimSpace(d))
		if d != "" {
			out = append(out, d)
		}
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func handleDictionary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		word := strings.TrimSpace(strings.Join(args, " "))
		if word == "" {
			s.Reply(info, dictionaryGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING DICTIONARY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		var b strings.Builder
		b.WriteString("*🔰 DICTIONARY 🔰*\n\n")
		b.WriteString("*📖 WORD ❯ " + strings.ToUpper(word) + "*\n")

		// primary: Wiktionary
		if _, entries, ok := dictWiktionary(ctx, word); ok {
			for _, e := range entries {
				b.WriteString("\n*▪️ " + strings.ToUpper(e.POS) + "*\n")
				for _, d := range e.Defs {
					b.WriteString("*" + d + "*\n")
				}
			}
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}

		// fallback: Datamuse
		if defs, ok := dictDatamuse(ctx, word); ok {
			b.WriteString("\n*▪️ MEANING*\n")
			for _, d := range defs {
				b.WriteString("*" + d + "*\n")
			}
			s.Reply(info, strings.TrimSpace(b.String()))
			return
		}

		if !ctxTimedOut(ctx) {
			s.Reply(info, "*🔰 WORD NOT FOUND, PLEASE CHECK THE SPELLING*")
		}
	})
}

// ============================================================================
// .CURRENCY — LIVE CURRENCY CONVERSION
// ============================================================================

func currencyGuide(prefix string) string {
	return "*🔰 CURRENCY CONVERTER 🔰*\n\n" +
		"*CONVERT ANY CURRENCY TO ANY CURRENCY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "CURRENCY <AMOUNT> <FROM> <TO> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CURRENCY 100 USD PKR ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "CURRENCY 5000 PKR USD ❯*"
}

func handleCurrency(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		if len(args) < 3 {
			s.Reply(info, currencyGuide(prefix))
			return
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(args[0]), 64)
		if err != nil {
			s.Reply(info, currencyGuide(prefix))
			return
		}
		from := strings.ToUpper(strings.TrimSpace(args[1]))
		to := strings.ToUpper(strings.TrimSpace(args[2]))
		waitID := s.ReplyWithID(info, "*CONVERTING CURRENCY....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://open.er-api.com/v6/latest/" + url.PathEscape(from)
		var res struct {
			Result   string             `json:"result"`
			BaseCode string             `json:"base_code"`
			Rates    map[string]float64 `json:"rates"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Result != "success" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID CURRENCY CODE, PLEASE CHECK*")
			}
			return
		}
		rate, ok := res.Rates[to]
		if !ok {
			s.Reply(info, "*🔰 TARGET CURRENCY NOT FOUND*")
			return
		}
		converted := amount * rate
		var b strings.Builder
		b.WriteString("*🔰 CURRENCY CONVERTER 🔰*\n\n")
		b.WriteString("*💵 FROM ❯ " + fmt.Sprintf("%.2f", amount) + " " + from + "*\n")
		b.WriteString("*💱 RATE ❯ 1 " + from + " = " + fmt.Sprintf("%.4f", rate) + " " + to + "*\n")
		b.WriteString("*✅ RESULT ❯ " + fmt.Sprintf("%.2f", converted) + " " + to + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .TIMEZONE — CURRENT TIME IN ANY TIMEZONE
// ============================================================================

func timezoneGuide(prefix string) string {
	return "*🔰 WORLD CLOCK 🔰*\n\n" +
		"*GET THE CURRENT TIME IN ANY TIMEZONE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TIMEZONE <ZONE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIMEZONE ASIA/KARACHI ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TIMEZONE AMERICA/NEW_YORK ❯*"
}

func handleTimezone(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		zone := strings.TrimSpace(strings.Join(args, " "))
		if zone == "" {
			s.Reply(info, timezoneGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*CHECKING TIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://timeapi.io/api/Time/current/zone?timeZone=" + url.QueryEscape(zone)
		var res struct {
			Year      int    `json:"year"`
			Month     int    `json:"month"`
			Day       int    `json:"day"`
			Hour      int    `json:"hour"`
			Minute    int    `json:"minute"`
			Seconds   int    `json:"seconds"`
			DayOfWeek string `json:"dayOfWeek"`
			TimeZone  string `json:"timeZone"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.TimeZone == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 INVALID TIMEZONE, PLEASE CHECK*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 WORLD CLOCK 🔰*\n\n")
		b.WriteString("*🌍 ZONE ❯ " + strings.ToUpper(res.TimeZone) + "*\n")
		b.WriteString("*📅 DATE ❯ " + fmt.Sprintf("%02d-%02d-%d", res.Day, res.Month, res.Year) + "*\n")
		b.WriteString("*📆 DAY ❯ " + strings.ToUpper(res.DayOfWeek) + "*\n")
		b.WriteString("*🕐 TIME ❯ " + fmt.Sprintf("%02d:%02d:%02d", res.Hour, res.Minute, res.Seconds) + "*")
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .NEWS — TOP NEWS HEADLINES
// ============================================================================

func newsGuide(prefix string) string {
	return "*🔰 NEWS HEADLINES 🔰*\n\n" +
		"*GET THE LATEST NEWS OF ANY COUNTRY OR CITY*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "NEWS ❯*\n" +
		"*TOP WORLD NEWS HEADLINES*\n\n" +
		"*❮ " + prefix + "NEWS <COUNTRY> ❯*\n" +
		"*LATEST NEWS OF THAT COUNTRY*\n" +
		"*EXAMPLE ❮ " + prefix + "NEWS PAKISTAN ❯*\n\n" +
		"*❮ " + prefix + "NEWS <COUNTRY> <CITY> ❯*\n" +
		"*LATEST NEWS OF THAT CITY*\n" +
		"*EXAMPLE ❮ " + prefix + "NEWS PAKISTAN ISLAMABAD ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "NEWS INDIA DELHI ❯*"
}

// newsRSS is the minimal shape of a Bing News RSS feed (has descriptions).
type newsRSS struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

// newsCleanTitle strips the trailing " - Source" that news feeds append.
func newsCleanTitle(t string) string {
	t = strings.TrimSpace(t)
	if i := strings.LastIndex(t, " - "); i > 0 {
		t = strings.TrimSpace(t[:i])
	}
	return t
}

// newsFetchPage pulls a single Bing News RSS page (offset = "first" param) and
// returns the items as (title, summary) pairs. Bing provides real English
// summaries, so we never have to dump raw links.
func newsFetchPage(ctx context.Context, query string, offset int) [][2]string {
	u := "https://www.bing.com/news/search?q=" + url.QueryEscape(query) +
		"&format=RSS&first=" + strconv.Itoa(offset)
	raw, err := funGetBytes(ctx, u)
	if err != nil {
		return nil
	}
	var feed newsRSS
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil
	}
	out := make([][2]string, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		title := newsCleanTitle(it.Title)
		if title == "" {
			continue
		}
		out = append(out, [2]string{title, stripHTML(it.Description)})
	}
	return out
}

// newsFetch builds a LARGE pool of headlines by pulling several Bing News RSS
// pages in parallel, de-duplicates them, then returns a RANDOM selection of
// `max` items. Because the pool is far bigger than what we show and the pick
// is shuffled on every call, each invocation returns FRESH news instead of the
// same top headlines every time.
func newsFetch(ctx context.Context, query string, max int) ([][2]string, bool) {
	// Bing paginates ~8-9 items per page; grab several pages for a big pool.
	offsets := []int{1, 9, 17, 25, 33, 41, 49, 57}

	var (
		mu   sync.Mutex
		pool [][2]string
		seen = map[string]bool{}
		wg   sync.WaitGroup
	)
	for _, off := range offsets {
		wg.Add(1)
		go func(off int) {
			defer wg.Done()
			items := newsFetchPage(ctx, query, off)
			mu.Lock()
			for _, it := range items {
				key := strings.ToLower(it[0])
				if seen[key] {
					continue
				}
				seen[key] = true
				pool = append(pool, it)
			}
			mu.Unlock()
		}(off)
	}
	wg.Wait()

	if len(pool) == 0 {
		return nil, false
	}

	// Shuffle the whole pool so every call surfaces a different slice.
	rand.Shuffle(len(pool), func(i, j int) {
		pool[i], pool[j] = pool[j], pool[i]
	})

	if len(pool) > max {
		pool = pool[:max]
	}
	return pool, true
}

func handleNews(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		// ── .news (no args) → guide ──
		if len(args) == 0 {
			s.Reply(info, newsGuide(prefix))
			return
		}

		// ── .news <country> [city] → location news ──
		country := strings.TrimSpace(args[0])
		city := ""
		if len(args) >= 2 {
			city = strings.TrimSpace(strings.Join(args[1:], " "))
		}
		query := country
		label := strings.ToUpper(country)
		if city != "" {
			query = country + " " + city
			label = strings.ToUpper(city) + ", " + strings.ToUpper(country)
		}

		waitID := s.ReplyWithID(info, "*FETCHING NEWS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		items, ok := newsFetch(ctx, query, 8)
		if !ok {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "NEWS")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 NEWS HEADLINES 🔰*\n\n")
		b.WriteString("*📰 LOCATION ❯ " + label + "*\n\n")
		for i, it := range items {
			b.WriteString("*" + strconv.Itoa(i+1) + ". " + it[0] + "*\n")
			if it[1] != "" {
				b.WriteString(it[1] + "\n")
			}
			b.WriteString("\n")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ============================================================================
// .LYRICS — SONG LYRICS
// ============================================================================

func lyricsGuide(prefix string) string {
	return "*🔰 SONG LYRICS 🔰*\n\n" +
		"*GET THE FULL LYRICS OF ANY SONG*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "LYRICS <SONG NAME> ❯*\n" +
		"*JUST TYPE THE SONG NAME — THE BOT FINDS THE ARTIST ITSELF*\n\n" +
		"*EXAMPLE ❮ " + prefix + "LYRICS YELLOW ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "LYRICS TUM HI HO ❯*"
}

func handleLyrics(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		raw := strings.TrimSpace(strings.Join(args, " "))
		if raw == "" {
			s.Reply(info, lyricsGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING LYRICS....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		// Step 1: auto-detect the song + artist from just the song name.
		// If the user typed "artist - song", honour that split directly.
		artist := ""
		song := raw
		if parts := strings.SplitN(raw, "-", 2); len(parts) == 2 {
			if a := strings.TrimSpace(parts[0]); a != "" {
				artist = a
				song = strings.TrimSpace(parts[1])
			}
		}

		if artist == "" {
			su := "https://api.lyrics.ovh/suggest/" + url.PathEscape(raw)
			var sug struct {
				Data []struct {
					Title  string `json:"title"`
					Artist struct {
						Name string `json:"name"`
					} `json:"artist"`
				} `json:"data"`
			}
			if err := funGetJSON(ctx, su, &sug); err == nil && len(sug.Data) > 0 {
				song = sug.Data[0].Title
				artist = sug.Data[0].Artist.Name
			}
		}
		if song == "" {
			song = raw
		}

		// Step 2: fetch the lyrics. Try the detected artist first, then a
		// couple of fallbacks so a wrong guess never blocks the result.
		fetch := func(a, t string) string {
			if a == "" || t == "" {
				return ""
			}
			u := "https://api.lyrics.ovh/v1/" + url.PathEscape(a) + "/" + url.PathEscape(t)
			var res struct {
				Lyrics string `json:"lyrics"`
			}
			if err := funGetJSON(ctx, u, &res); err != nil {
				return ""
			}
			return strings.TrimSpace(res.Lyrics)
		}

		lyr := fetch(artist, song)
		if lyr == "" {
			lyr = fetch(artist, raw)
		}
		if lyr == "" {
			lyr = fetch("", song)
		}
		if lyr == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 LYRICS NOT FOUND, PLEASE CHECK THE SONG NAME*")
			}
			return
		}
		if len(lyr) > 3500 {
			lyr = lyr[:3500] + "\n\n...(TRUNCATED)"
		}
		var b strings.Builder
		b.WriteString("*🔰 SONG LYRICS 🔰*\n\n")
		b.WriteString("*🎵 " + strings.ToUpper(song) + "*\n")
		if artist != "" {
			b.WriteString("*🎤 " + strings.ToUpper(artist) + "*\n")
		}
		b.WriteString("\n")
		b.WriteString(lyr)
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .GITHUB — GITHUB USER PROFILE
// ============================================================================

func githubGuide(prefix string) string {
	return "*🔰 GITHUB LOOKUP 🔰*\n\n" +
		"*GET THE PROFILE OF ANY GITHUB USER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GITHUB <USERNAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "GITHUB TORVALDS ❯*"
}

func handleGithub(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		user := strings.TrimSpace(strings.Join(args, " "))
		if user == "" {
			s.Reply(info, githubGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING GITHUB PROFILE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://api.github.com/users/" + url.PathEscape(user)
		var res struct {
			Login       string `json:"login"`
			Name        string `json:"name"`
			Bio         string `json:"bio"`
			Company     string `json:"company"`
			Location    string `json:"location"`
			Blog        string `json:"blog"`
			PublicRepos int    `json:"public_repos"`
			Followers   int    `json:"followers"`
			Following   int    `json:"following"`
			HTMLURL     string `json:"html_url"`
			AvatarURL   string `json:"avatar_url"`
			Message     string `json:"message"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Login == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 GITHUB USER NOT FOUND*")
			}
			return
		}

		// trim every value so no stray space ever sits before the closing *
		// (a trailing space breaks WhatsApp bold rendering).
		clean := func(v string) string {
			return strings.TrimSpace(strings.ReplaceAll(v, "\n", " "))
		}

		var b strings.Builder
		b.WriteString("*🔰 GITHUB PROFILE 🔰*\n\n")
		b.WriteString("*👤 USER ❯ " + clean(res.Login) + "*\n")
		if n := clean(res.Name); n != "" {
			b.WriteString("*📛 NAME ❯ " + n + "*\n")
		}
		if c := clean(res.Company); c != "" {
			b.WriteString("*🏢 COMPANY ❯ " + c + "*\n")
		}
		if l := clean(res.Location); l != "" {
			b.WriteString("*📍 LOCATION ❯ " + l + "*\n")
		}
		b.WriteString("*📦 REPOS ❯ " + strconv.Itoa(res.PublicRepos) + "*\n")
		b.WriteString("*👥 FOLLOWERS ❯ " + strconv.Itoa(res.Followers) + "*\n")
		b.WriteString("*➡️ FOLLOWING ❯ " + strconv.Itoa(res.Following) + "*\n")
		b.WriteString("*🔗 " + clean(res.HTMLURL) + "*")

		// Bio goes at the very END, raw (not bolded) so multi-line bios and
		// bullet lists render exactly as the user wrote them.
		if bio := strings.TrimSpace(res.Bio); bio != "" {
			bio = strings.ReplaceAll(bio, "\r\n", "\n")
			bio = strings.ReplaceAll(bio, "\r", "\n")
			b.WriteString("\n\n*BIO IS HERE*\n")
			b.WriteString(bio)
		}

		caption := b.String()

		// Try to attach the profile picture. If it can't be fetched, still
		// send the text so the command never stalls on a missing avatar.
		if av := strings.TrimSpace(res.AvatarURL); av != "" {
			if img, err := funGetBytes(ctx, av); err == nil && len(img) > 0 {
				if sendErr := s.SendImage(info, img, caption); sendErr == nil {
					return
				}
			}
		}
		s.Reply(info, caption)
	})
}

// ============================================================================
// .ANIME — ANIME INFO
// ============================================================================

func animeGuide(prefix string) string {
	return "*🔰 ANIME INFO 🔰*\n\n" +
		"*GET DETAILS OF ANY ANIME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "ANIME <TITLE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "ANIME NARUTO ❯*"
}

// animeResult is the normalised anime record both sources fill in.
type animeResult struct {
	Title    string
	Synopsis string
	Episodes int
	Score    float64
	Status   string
	Type     string
	Year     int
	URL      string
}

// animeFromAniList queries the AniList GraphQL API (reliable, no gzip issue)
// and returns the most popular matching anime.
func animeFromAniList(ctx context.Context, q string) (animeResult, bool) {
	body := []byte(`{"query":"query($s:String){Page(perPage:5){media(search:$s,type:ANIME,sort:SEARCH_MATCH){title{romaji english} description(asHtml:false) episodes averageScore status format startDate{year} siteUrl popularity}}}","variables":{"s":` + strconv.Quote(q) + `}}`)
	var res struct {
		Data struct {
			Page struct {
				Media []struct {
					Title struct {
						Romaji  string `json:"romaji"`
						English string `json:"english"`
					} `json:"title"`
					Description  string `json:"description"`
					Episodes     int    `json:"episodes"`
					AverageScore int    `json:"averageScore"`
					Status       string `json:"status"`
					Format       string `json:"format"`
					Popularity   int    `json:"popularity"`
					SiteURL      string `json:"siteUrl"`
					StartDate    struct {
						Year int `json:"year"`
					} `json:"startDate"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if err := funPostJSON(ctx, "https://graphql.anilist.co", body, &res); err != nil {
		return animeResult{}, false
	}
	media := res.Data.Page.Media
	if len(media) == 0 {
		return animeResult{}, false
	}
	// pick the most popular match so "demon slayer" resolves to Kimetsu no
	// Yaiba rather than an obscure same-named short.
	best := media[0]
	for _, m := range media[1:] {
		if m.Popularity > best.Popularity {
			best = m
		}
	}
	title := best.Title.English
	if title == "" {
		title = best.Title.Romaji
	}
	return animeResult{
		Title:    title,
		Synopsis: stripHTML(best.Description),
		Episodes: best.Episodes,
		Score:    float64(best.AverageScore) / 10.0,
		Status:   best.Status,
		Type:     best.Format,
		Year:     best.StartDate.Year,
		URL:      best.SiteURL,
	}, true
}

// animeFromJikan is the fallback source (MyAnimeList via Jikan).
func animeFromJikan(ctx context.Context, q string) (animeResult, bool) {
	u := "https://api.jikan.moe/v4/anime?q=" + url.QueryEscape(q) + "&limit=1"
	var res struct {
		Data []struct {
			Title    string  `json:"title"`
			Synopsis string  `json:"synopsis"`
			Episodes int     `json:"episodes"`
			Score    float64 `json:"score"`
			Status   string  `json:"status"`
			Type     string  `json:"type"`
			URL      string  `json:"url"`
			Aired    struct {
				Prop struct {
					From struct {
						Year int `json:"year"`
					} `json:"from"`
				} `json:"prop"`
			} `json:"aired"`
		} `json:"data"`
	}
	if err := funGetJSONNoGzip(ctx, u, &res); err != nil || len(res.Data) == 0 {
		return animeResult{}, false
	}
	a := res.Data[0]
	return animeResult{
		Title:    a.Title,
		Synopsis: a.Synopsis,
		Episodes: a.Episodes,
		Score:    a.Score,
		Status:   a.Status,
		Type:     a.Type,
		Year:     a.Aired.Prop.From.Year,
		URL:      a.URL,
	}, true
}

func handleAnime(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, animeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING ANIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		a, ok := animeFromAniList(ctx, q)
		if !ok && !ctxTimedOut(ctx) {
			a, ok = animeFromJikan(ctx, q)
		}
		if !ok {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 ANIME NOT FOUND, PLEASE CHECK THE TITLE*")
			}
			return
		}

		syn := strings.TrimSpace(a.Synopsis)
		if len(syn) > 900 {
			syn = syn[:900] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 ANIME INFO 🔰*\n\n")
		b.WriteString("*🎬 TITLE ❯ " + strings.ToUpper(a.Title) + "*\n")
		if a.Type != "" {
			b.WriteString("*📺 TYPE ❯ " + a.Type + "*\n")
		}
		if a.Episodes > 0 {
			b.WriteString("*🔢 EPISODES ❯ " + strconv.Itoa(a.Episodes) + "*\n")
		}
		if a.Score > 0 {
			b.WriteString("*⭐ SCORE ❯ " + fmt.Sprintf("%.2f", a.Score) + "/10*\n")
		}
		if a.Status != "" {
			b.WriteString("*📡 STATUS ❯ " + strings.ToUpper(a.Status) + "*\n")
		}
		if a.Year > 0 {
			b.WriteString("*📅 YEAR ❯ " + strconv.Itoa(a.Year) + "*\n")
		}
		if syn != "" {
			b.WriteString("\n*📖 SYNOPSIS:*\n" + syn + "\n")
		}
		if a.URL != "" {
			b.WriteString("\n*🔗 " + a.URL + "*")
		}
		s.Reply(info, b.String())
	})
}

// ============================================================================
// .POKEMON — POKEMON INFO
// ============================================================================

func pokemonGuide(prefix string) string {
	return "*🔰 POKEDEX 🔰*\n\n" +
		"*GET DETAILS OF ANY POKEMON*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "POKEMON <NAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "POKEMON PIKACHU ❯*"
}

func handlePokemon(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		name := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		if name == "" {
			s.Reply(info, pokemonGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING POKEDEX....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		u := "https://pokeapi.co/api/v2/pokemon/" + url.PathEscape(name)
		var res struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Height int    `json:"height"`
			Weight int    `json:"weight"`
			Types  []struct {
				Type struct {
					Name string `json:"name"`
				} `json:"type"`
			} `json:"types"`
			Abilities []struct {
				Ability struct {
					Name string `json:"name"`
				} `json:"ability"`
			} `json:"abilities"`
			Stats []struct {
				BaseStat int `json:"base_stat"`
				Stat     struct {
					Name string `json:"name"`
				} `json:"stat"`
			} `json:"stats"`
		}
		if err := funGetJSON(ctx, u, &res); err != nil || res.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 POKEMON NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 POKEDEX 🔰*\n\n")
		b.WriteString("*🎮 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		b.WriteString("*🔢 ID ❯ #" + strconv.Itoa(res.ID) + "*\n")
		b.WriteString("*📏 HEIGHT ❯ " + fmt.Sprintf("%.1f", float64(res.Height)/10) + " M*\n")
		b.WriteString("*⚖️ WEIGHT ❯ " + fmt.Sprintf("%.1f", float64(res.Weight)/10) + " KG*\n")
		if len(res.Types) > 0 {
			ts := make([]string, 0, len(res.Types))
			for _, t := range res.Types {
				ts = append(ts, strings.ToUpper(t.Type.Name))
			}
			b.WriteString("*🧬 TYPE ❯ " + strings.Join(ts, ", ") + "*\n")
		}
		if len(res.Abilities) > 0 {
			as := make([]string, 0, len(res.Abilities))
			for _, a := range res.Abilities {
				as = append(as, strings.ToUpper(a.Ability.Name))
			}
			b.WriteString("*✨ ABILITIES ❯ " + strings.Join(as, ", ") + "*\n")
		}
		if len(res.Stats) > 0 {
			b.WriteString("\n*📊 BASE STATS:*\n")
			for _, st := range res.Stats {
				b.WriteString("*▪️ " + strings.ToUpper(st.Stat.Name) + " ❯ " + strconv.Itoa(st.BaseStat) + "*\n")
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ============================================================================
// REGISTRATION
// ============================================================================

func init() {
	Register(Command{Name: "prayer", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET TODAY'S PRAYER (NAMAZ) TIMES OF ANY CITY. USE IT AS .PRAYER <CITY>.", Run: handlePrayer})
	Register(Command{Name: "quran", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO READ THE HOLY QURAN WITH URDU AND ENGLISH TARJUMA. USE IT AS .QURAN <SURAH> OR .QURAN <SURAH> <AYAH>.", Run: handleQuran})
	Register(Command{Name: "dictionary", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE MEANING OF ANY ENGLISH WORD. USE IT AS .DICTIONARY <WORD>.", Run: handleDictionary})
	Register(Command{Name: "currency", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO CONVERT ANY CURRENCY TO ANY CURRENCY. USE IT AS .CURRENCY <AMOUNT> <FROM> <TO>.", Run: handleCurrency})
	Register(Command{Name: "timezone", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE CURRENT TIME IN ANY TIMEZONE. USE IT AS .TIMEZONE <ZONE>.", Run: handleTimezone})
	Register(Command{Name: "news", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE LATEST NEWS OF ANY COUNTRY OR CITY. USE IT AS .NEWS <COUNTRY> OR .NEWS <COUNTRY> <CITY>.", Run: handleNews})
	Register(Command{Name: "lyrics", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE FULL LYRICS OF ANY SONG. JUST TYPE THE SONG NAME, USE IT AS .LYRICS <SONG NAME>.", Run: handleLyrics})
	Register(Command{Name: "github", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET THE PROFILE OF ANY GITHUB USER. USE IT AS .GITHUB <USERNAME>.", Run: handleGithub})
	Register(Command{Name: "anime", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DETAILS OF ANY ANIME. USE IT AS .ANIME <TITLE>.", Run: handleAnime})
	Register(Command{Name: "pokemon", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET DETAILS OF ANY POKEMON. USE IT AS .POKEMON <NAME>.", Run: handlePokemon})

	// ── short aliases (hidden — menu me sirf long form dikhta hai, same work) ──
	Register(Command{Name: "namaz", Category: "TOOLS", Desc: "Short alias of .prayer", Hidden: true, Run: handlePrayer})
	Register(Command{Name: "salah", Category: "TOOLS", Desc: "Short alias of .prayer", Hidden: true, Run: handlePrayer})
	Register(Command{Name: "ayah", Category: "TOOLS", Desc: "Short alias of .quran", Hidden: true, Run: handleQuran})
	Register(Command{Name: "dict", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "meaning", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "define", Category: "TOOLS", Desc: "Short alias of .dictionary", Hidden: true, Run: handleDictionary})
	Register(Command{Name: "rate", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "exchange", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "forex", Category: "TOOLS", Desc: "Short alias of .currency", Hidden: true, Run: handleCurrency})
	Register(Command{Name: "time", Category: "TOOLS", Desc: "Short alias of .timezone", Hidden: true, Run: handleTimezone})
	Register(Command{Name: "worldtime", Category: "TOOLS", Desc: "Short alias of .timezone", Hidden: true, Run: handleTimezone})
	Register(Command{Name: "headlines", Category: "TOOLS", Desc: "Short alias of .news", Hidden: true, Run: handleNews})
	Register(Command{Name: "lyric", Category: "TOOLS", Desc: "Short alias of .lyrics", Hidden: true, Run: handleLyrics})
	Register(Command{Name: "ghuser", Category: "TOOLS", Desc: "Short alias of .github", Hidden: true, Run: handleGithub})
	Register(Command{Name: "gituser", Category: "TOOLS", Desc: "Short alias of .github", Hidden: true, Run: handleGithub})
	Register(Command{Name: "animeinfo", Category: "TOOLS", Desc: "Short alias of .anime", Hidden: true, Run: handleAnime})
	Register(Command{Name: "pokedex", Category: "TOOLS", Desc: "Short alias of .pokemon", Hidden: true, Run: handlePokemon})
	Register(Command{Name: "poke", Category: "TOOLS", Desc: "Short alias of .pokemon", Hidden: true, Run: handlePokemon})
}
