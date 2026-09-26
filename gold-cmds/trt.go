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
	"unicode/utf8"

	"go.mau.fi/whatsmeow/types"
)

// trtLang is one supported language (code + display name).
type trtLang struct {
	Code string
	Name string
}

// trtLangs is the full Google Translate NMT language list (code -> name).
// Sourced from Google's own live language endpoint, which reports 249
// languages; three legacy codes it no longer lists (fil/he/jv) are kept
// because the translate endpoint still accepts them. Order is
// alphabetical by name for a clean guidance message.
var trtLangs = []trtLang{
	{"ab", "ABKHAZ"}, {"ace", "ACEHNESE"}, {"ach", "ACHOLI"}, {"aa", "AFAR"},
	{"af", "AFRIKAANS"}, {"ak", "AKAN"}, {"sq", "ALBANIAN"}, {"alz", "ALUR"},
	{"am", "AMHARIC"}, {"ar", "ARABIC"}, {"hy", "ARMENIAN"}, {"as", "ASSAMESE"},
	{"av", "AVAR"}, {"awa", "AWADHI"}, {"ay", "AYMARA"}, {"az", "AZERBAIJANI"},
	{"ban", "BALINESE"}, {"bal", "BALOCHI"}, {"bm", "BAMBARA"}, {"bci", "BAOULÉ"},
	{"ba", "BASHKIR"}, {"eu", "BASQUE"}, {"btx", "BATAK KARO"},
	{"bts", "BATAK SIMALUNGUN"}, {"bbc", "BATAK TOBA"}, {"be", "BELARUSIAN"},
	{"bem", "BEMBA"}, {"bn", "BENGALI"}, {"bew", "BETAWI"}, {"bho", "BHOJPURI"},
	{"bik", "BIKOL"}, {"bs", "BOSNIAN"}, {"br", "BRETON"}, {"bg", "BULGARIAN"},
	{"bua", "BURYAT"}, {"yue", "CANTONESE"}, {"ca", "CATALAN"}, {"ceb", "CEBUANO"},
	{"ch", "CHAMORRO"}, {"ce", "CHECHEN"}, {"ny", "CHICHEWA"},
	{"zh-CN", "CHINESE (SIMPLIFIED)"}, {"zh-TW", "CHINESE (TRADITIONAL)"},
	{"chk", "CHUUKESE"}, {"cv", "CHUVASH"}, {"co", "CORSICAN"},
	{"crh", "CRIMEAN TATAR (CYRILLIC)"}, {"crh-Latn", "CRIMEAN TATAR (LATIN)"},
	{"hr", "CROATIAN"}, {"cs", "CZECH"}, {"da", "DANISH"}, {"fa-AF", "DARI"},
	{"din", "DINKA"}, {"dv", "DIVEHI"}, {"doi", "DOGRI"}, {"dov", "DOMBE"},
	{"nl", "DUTCH"}, {"dyu", "DYULA"}, {"dz", "DZONGKHA"}, {"en", "ENGLISH"},
	{"eo", "ESPERANTO"}, {"et", "ESTONIAN"}, {"ee", "EWE"}, {"fo", "FAROESE"},
	{"fj", "FIJIAN"}, {"fil", "FILIPINO"}, {"fi", "FINNISH"}, {"fon", "FON"},
	{"fr", "FRENCH"}, {"fr-CA", "FRENCH (CANADA)"}, {"fy", "FRISIAN"}, {"fur", "FRIULIAN"},
	{"ff", "FULANI"}, {"gaa", "GA"}, {"gl", "GALICIAN"}, {"ka", "GEORGIAN"},
	{"de", "GERMAN"}, {"el", "GREEK"}, {"gn", "GUARANI"}, {"gu", "GUJARATI"},
	{"ht", "HAITIAN CREOLE"}, {"cnh", "HAKHA CHIN"}, {"ha", "HAUSA"}, {"haw", "HAWAIIAN"},
	{"he", "HEBREW"}, {"iw", "HEBREW (IW)"}, {"hil", "HILIGAYNON"}, {"hi", "HINDI"},
	{"hmn", "HMONG"}, {"hu", "HUNGARIAN"}, {"hrx", "HUNSRIK"}, {"iba", "IBAN"},
	{"is", "ICELANDIC"}, {"ig", "IGBO"}, {"ilo", "ILOKO"}, {"id", "INDONESIAN"},
	{"iu-Latn", "INUKTUT (LATIN)"}, {"iu", "INUKTUT (SYLLABICS)"}, {"ga", "IRISH"},
	{"it", "ITALIAN"}, {"jam", "JAMAICAN PATOIS"}, {"ja", "JAPANESE"}, {"jv", "JAVANESE"},
	{"jw", "JAVANESE (JW)"}, {"kac", "JINGPO"}, {"kl", "KALAALLISUT"}, {"kn", "KANNADA"},
	{"kr", "KANURI"}, {"pam", "KAPAMPANGAN"}, {"kk", "KAZAKH"}, {"kha", "KHASI"},
	{"km", "KHMER"}, {"cgg", "KIGA"}, {"kg", "KIKONGO"}, {"rw", "KINYARWANDA"},
	{"ktu", "KITUBA"}, {"trp", "KOKBOROK"}, {"kv", "KOMI"}, {"gom", "KONKANI"},
	{"ko", "KOREAN"}, {"kri", "KRIO"}, {"ku", "KURDISH (KURMANJI)"},
	{"ckb", "KURDISH (SORANI)"}, {"ky", "KYRGYZ"}, {"lo", "LAO"}, {"ltg", "LATGALIAN"},
	{"la", "LATIN"}, {"lv", "LATVIAN"}, {"lij", "LIGURIAN"}, {"li", "LIMBURGISH"},
	{"ln", "LINGALA"}, {"lt", "LITHUANIAN"}, {"lmo", "LOMBARD"}, {"lg", "LUGANDA"},
	{"luo", "LUO"}, {"lb", "LUXEMBOURGISH"}, {"mk", "MACEDONIAN"}, {"mad", "MADURESE"},
	{"mai", "MAITHILI"}, {"mak", "MAKASSAR"}, {"mg", "MALAGASY"}, {"ms", "MALAY"},
	{"ms-Arab", "MALAY (JAWI)"}, {"ml", "MALAYALAM"}, {"mt", "MALTESE"}, {"mam", "MAM"},
	{"gv", "MANX"}, {"mi", "MAORI"}, {"mr", "MARATHI"}, {"mh", "MARSHALLESE"},
	{"mwr", "MARWADI"}, {"mfe", "MAURITIAN CREOLE"}, {"chm", "MEADOW MARI"},
	{"mni-Mtei", "MEITEILON (MANIPURI)"}, {"min", "MINANG"}, {"lus", "MIZO"},
	{"mn", "MONGOLIAN"}, {"my", "MYANMAR (BURMESE)"},
	{"nhe", "NAHUATL (EASTERN HUASTECA)"}, {"ndc-ZW", "NDAU"}, {"nr", "NDEBELE (SOUTH)"},
	{"new", "NEPALBHASA (NEWARI)"}, {"ne", "NEPALI"}, {"bm-Nkoo", "NKO"},
	{"no", "NORWEGIAN"}, {"nus", "NUER"}, {"oc", "OCCITAN"}, {"or", "ODIA (ORIYA)"},
	{"om", "OROMO"}, {"os", "OSSETIAN"}, {"pag", "PANGASINAN"}, {"pap", "PAPIAMENTO"},
	{"ps", "PASHTO"}, {"fa", "PERSIAN"}, {"pl", "POLISH"}, {"pt", "PORTUGUESE"},
	{"pt-PT", "PORTUGUESE (PORTUGAL)"}, {"pa", "PUNJABI"},
	{"pa-Arab", "PUNJABI (SHAHMUKHI)"}, {"qu", "QUECHUA"}, {"kek", "QʼEQCHIʼ"},
	{"rom", "ROMANI"}, {"ro", "ROMANIAN"}, {"rn", "RUNDI"}, {"ru", "RUSSIAN"},
	{"se", "SAMI (NORTH)"}, {"sm", "SAMOAN"}, {"sg", "SANGO"}, {"sa", "SANSKRIT"},
	{"sat", "SANTALI"}, {"sat-Latn", "SANTALI (LATIN)"}, {"gd", "SCOTS GAELIC"},
	{"nso", "SEPEDI"}, {"sr", "SERBIAN"}, {"st", "SESOTHO"}, {"crs", "SEYCHELLOIS CREOLE"},
	{"shn", "SHAN"}, {"sn", "SHONA"}, {"scn", "SICILIAN"}, {"szl", "SILESIAN"},
	{"sd", "SINDHI"}, {"si", "SINHALA"}, {"sk", "SLOVAK"}, {"sl", "SLOVENIAN"},
	{"so", "SOMALI"}, {"es", "SPANISH"}, {"su", "SUNDANESE"}, {"sus", "SUSU"},
	{"sw", "SWAHILI"}, {"ss", "SWATI"}, {"sv", "SWEDISH"}, {"tl", "TAGALOG"},
	{"ty", "TAHITIAN"}, {"tg", "TAJIK"}, {"ber-Latn", "TAMAZIGHT"},
	{"ber", "TAMAZIGHT (TIFINAGH)"}, {"ta", "TAMIL"}, {"tt", "TATAR"}, {"te", "TELUGU"},
	{"tet", "TETUM"}, {"th", "THAI"}, {"bo", "TIBETAN"}, {"ti", "TIGRINYA"},
	{"tiv", "TIV"}, {"tpi", "TOK PISIN"}, {"to", "TONGAN"}, {"lua", "TSHILUBA"},
	{"ts", "TSONGA"}, {"tn", "TSWANA"}, {"tcy", "TULU"}, {"tum", "TUMBUKA"},
	{"tr", "TURKISH"}, {"tk", "TURKMEN"}, {"tyv", "TUVAN"}, {"udm", "UDMURT"},
	{"uk", "UKRAINIAN"}, {"ur", "URDU"}, {"ug", "UYGHUR"}, {"uz", "UZBEK"},
	{"ve", "VENDA"}, {"vec", "VENETIAN"}, {"vi", "VIETNAMESE"}, {"war", "WARAY"},
	{"cy", "WELSH"}, {"wo", "WOLOF"}, {"xh", "XHOSA"}, {"sah", "YAKUT"}, {"yi", "YIDDISH"},
	{"yo", "YORUBA"}, {"yua", "YUCATEC MAYA"}, {"zap", "ZAPOTEC"}, {"zu", "ZULU"},
}

// trtRegionAliases maps a COUNTRY / CITY / TOWN / VILLAGE / DIALECT name to the
// closest language Google actually supports. This is how "har city, har gaon"
// is covered: Google has ~130 language codes, so a region that speaks a dialect
// (Saraiki, Hindko, Marwari, ...) resolves to its nearest supported language,
// and a city resolves to the dominant language of that place. Owner order:
// a user types the place or dialect they speak and the bot replies in it.
//
// Keys are lower-case; values are trtLangs codes.
var trtRegionAliases = map[string]string{
	// ── Pakistan — provinces, cities, dialects ──
	// PAKISTANI PUNJAB IS SHAHMUKHI, NOT GURMUKHI: Google exposes Punjabi
	// twice — "pa" is Gurmukhi (Indian script) and "pa-Arab" is Shahmukhi
	// (Pakistani script). Every Punjab-Pakistan city/dialect below therefore
	// resolves to pa-Arab, so a Lahori or Multani user gets their own script
	// instead of the Indian one.
	"pakistan": "ur", "pakistani": "ur", "pak": "ur",
	"muhajir": "ur", "muhajiri": "ur", "mohajir": "ur",
	"punjabi (pakistan)": "pa-Arab", "lahnda": "pa-Arab", "western punjabi": "pa-Arab",
	"shahmukhi": "pa-Arab", "punjabi shahmukhi": "pa-Arab", "pakistani punjabi": "pa-Arab",
	"lahore": "pa-Arab", "lahori": "pa-Arab", "faisalabad": "pa-Arab", "gujranwala": "pa-Arab",
	"sialkot": "pa-Arab", "multan": "pa-Arab", "multani": "pa-Arab", "rawalpindi": "pa-Arab",
	"islamabad": "ur", "karachi": "ur", "karachite": "ur", "hyderabad (pakistan)": "ur",
	"peshawar": "ps", "peshawari": "ps", "khyber": "ps", "kpk": "ps",
	"quetta": "bal", "balochistan": "bal", "balochi": "bal", "brahui": "bal",
	"gilgit": "ur", "gilgiti": "ur", "skardu": "ur", "baltistan": "ur",
	"kashmir (pakistan)": "ur", "azad kashmir": "ur", "mirpur": "pa-Arab", "muzaffarabad": "ur",
	"saraiki": "pa-Arab", "seraiki": "pa-Arab", "siraiki": "pa-Arab", "riyasati": "pa-Arab",
	"hindko": "pa-Arab", "hindku": "pa-Arab", "pothwari": "pa-Arab", "potohari": "pa-Arab",
	"pahari": "pa-Arab", "jhangvi": "pa-Arab", "dera ghazi khan": "pa-Arab",
	"bahawalpur": "pa-Arab", "sargodha": "pa-Arab", "sahiwal": "pa-Arab", "okara": "pa-Arab",
	"chitrali": "ps", "khowar": "ps", "shina": "ur", "burushaski": "ur", "wakhi": "ps",
	"mewati": "hi", "haryanvi": "hi", "rangri": "hi",
	// ── India — states, cities, dialects ──
	"india": "hi", "indian": "hi", "hindustani": "hi",
	"delhi": "hi", "new delhi": "hi", "mumbai": "mr", "bombay": "mr",
	"pune": "mr", "nagpur": "mr", "maharashtra": "mr", "marathi": "mr",
	"bengaluru": "kn", "bangalore": "kn", "mysore": "kn", "karnataka": "kn", "kannada": "kn",
	"chennai": "ta", "madras": "ta", "tamil nadu": "ta", "tamil": "ta",
	"hyderabad (india)": "te", "telangana": "te", "telugu": "te", "andhra": "te",
	"kolkata": "bn", "calcutta": "bn", "west bengal": "bn", "bengali": "bn", "bangla": "bn",
	"ahmedabad": "gu", "gujarat": "gu", "gujarati": "gu", "surat": "gu",
	"kochi": "ml", "kerala": "ml", "malayalam": "ml", "trivandrum": "ml",
	"lucknow": "hi", "kanpur": "hi", "varanasi": "hi", "banaras": "hi", "agra": "hi",
	"jaipur": "hi", "rajasthan": "hi", "marwari": "hi", "marwadi": "hi", "mewari": "hi",
	"bhopal": "hi", "madhya pradesh": "hi", "indore": "hi", "chhattisgarhi": "hi",
	"awadhi": "hi", "brij": "hi", "braj": "hi", "magahi": "hi", "magadhi": "hi",
	"bhojpuri": "bho", "bihar": "bho", "patna": "bho", "maithili": "mai", "maithil": "mai",
	"dogri": "doi", "jammu": "doi", "konkani": "gom", "goa": "gom",
	"santali": "sat", "santhali": "sat", "manipuri": "mni-Mtei", "meitei": "mni-Mtei",
	"mizo": "lus", "assamese": "as", "assam": "as", "guwahati": "as",
	"odia": "or", "oriya": "or", "odisha": "or", "bhubaneswar": "or",
	"kashmiri": "ur", "koshur": "ur", "srinagar": "ur", "punjabi (india)": "pa",
	"amritsar": "pa", "chandigarh": "pa", "sindhi (india)": "sd",
	// ── South Asia neighbours ──
	"bangladesh": "bn", "dhaka": "bn", "chittagong": "bn",
	"nepal": "ne", "nepali": "ne", "kathmandu": "ne",
	"bhutan": "dz", "dzongkha": "dz",
	"sri lanka": "si", "sinhala": "si", "colombo": "si", "tamil (sri lanka)": "ta",
	"maldives": "dv", "dhivehi": "dv", "male": "dv",
	"afghanistan": "ps", "afghan": "ps", "kabul": "ps",
	"dari": "fa", "kandahar": "ps", "herat": "fa",
	// ── Middle East / Central Asia / Iranic ──
	"iran": "fa", "persian": "fa", "farsi": "fa", "tehran": "fa",
	"tajikistan": "tg", "tajiki": "tg", "dushanbe": "tg",
	"uzbekistan": "uz", "uzbek": "uz", "tashkent": "uz",
	"turkmenistan": "tk", "turkmen": "tk", "ashgabat": "tk",
	"kazakhstan": "kk", "kazakh": "kk", "almaty": "kk",
	"kyrgyzstan": "ky", "kyrgyz": "ky", "bishkek": "ky",
	"azerbaijan": "az", "azeri": "az", "baku": "az",
	"armenia": "hy", "armenian": "hy", "yerevan": "hy",
	"georgia": "ka", "georgian": "ka", "tbilisi": "ka",
	"turkey": "tr", "turkish": "tr", "istanbul": "tr", "ankara": "tr",
	"kurdistan": "ku", "kurmanji": "ku", "sorani": "ckb", "erbil": "ckb",
	"arabic": "ar", "saudi": "ar", "riyadh": "ar", "jeddah": "ar", "mecca": "ar",
	"uae": "ar", "dubai": "ar", "abu dhabi": "ar", "qatar": "ar", "doha": "ar",
	"kuwait": "ar", "bahrain": "ar", "oman": "ar", "muscat": "ar",
	"jordan": "ar", "amman": "ar", "lebanon": "ar", "beirut": "ar",
	"syria": "ar", "damascus": "ar", "iraq": "ar", "baghdad": "ar",
	"egypt": "ar", "cairo": "ar", "yemen": "ar", "sanaa": "ar",
	"morocco": "ar", "rabat": "ar", "casablanca": "ar", "algeria": "ar", "algiers": "ar",
	"tunisia": "ar", "tunis": "ar", "libya": "ar", "tripoli": "ar", "sudan": "ar", "khartoum": "ar",
	"israel": "he", "hebrew": "he", "tel aviv": "he", "jerusalem": "he",
	// ── Africa ──
	"nigeria": "yo", "lagos": "yo", "yoruba": "yo", "abuja": "ha", "hausa": "ha",
	"kano": "ha", "igbo": "ig", "enugu": "ig",
	"ethiopia": "am", "amharic": "am", "addis ababa": "am", "tigrinya": "ti",
	"kenya": "sw", "nairobi": "sw", "swahili": "sw", "kiswahili": "sw",
	"tanzania": "sw", "dar es salaam": "sw", "mombasa": "sw",
	"ghana": "ak", "accra": "ak", "akan": "ak", "twi": "ak",
	"uganda": "lg", "kampala": "lg", "luganda": "lg",
	"rwanda": "rw", "kigali": "rw", "kinyarwanda": "rw",
	"zimbabwe": "sn", "harare": "sn", "shona": "sn", "ndebele": "sn",
	"malawi": "ny", "lilongwe": "ny", "chichewa": "ny",
	"south africa": "zu", "zulu": "zu", "johannesburg": "zu", "durban": "zu",
	"cape town": "af", "afrikaans": "af", "pretoria": "nso", "sesotho": "st",
	"tsonga": "ts", "somalia": "so", "somali": "so", "mogadishu": "so",
	"senegal": "fr", "dakar": "fr", "mali": "bm", "bambara": "bm", "bamako": "bm",
	"congo": "ln", "kinshasa": "ln", "lingala": "ln", "madagascar": "mg", "malagasy": "mg",
	// ── Europe ──
	"uk": "en", "britain": "en", "england": "en", "london": "en", "usa": "en",
	"america": "en", "new york": "en", "ireland": "ga", "irish": "ga", "dublin": "ga",
	"wales": "cy", "welsh": "cy", "cardiff": "cy", "scotland": "gd", "scots gaelic": "gd",
	"france": "fr", "french": "fr", "paris": "fr", "belgium": "fr", "brussels": "fr",
	"spain": "es", "spanish": "es", "madrid": "es", "mexico": "es", "mexican": "es",
	"argentina": "es", "buenos aires": "es", "colombia": "es", "bogota": "es",
	"portugal": "pt", "portuguese": "pt", "lisbon": "pt", "brazil": "pt", "brasil": "pt",
	"italy": "it", "italian": "it", "rome": "it", "germany": "de", "german": "de",
	"berlin": "de", "austria": "de", "vienna": "de", "switzerland": "de", "zurich": "de",
	"netherlands": "nl", "dutch": "nl", "amsterdam": "nl", "holland": "nl",
	"russia": "ru", "russian": "ru", "moscow": "ru", "ukraine": "uk", "ukrainian": "uk",
	"kyiv": "uk", "poland": "pl", "polish": "pl", "warsaw": "pl",
	"greece": "el", "greek": "el", "athens": "el", "sweden": "sv", "swedish": "sv",
	"stockholm": "sv", "norway": "no", "norwegian": "no", "oslo": "no",
	"denmark": "da", "danish": "da", "copenhagen": "da", "finland": "fi", "finnish": "fi",
	"helsinki": "fi", "hungary": "hu", "hungarian": "hu", "budapest": "hu",
	"romania": "ro", "romanian": "ro", "bucharest": "ro", "bulgaria": "bg", "sofia": "bg",
	"serbia": "sr", "serbian": "sr", "belgrade": "sr", "croatia": "hr", "zagreb": "hr",
	"bosnia": "bs", "sarajevo": "bs", "slovakia": "sk", "slovenia": "sl",
	"czech": "cs", "czechia": "cs", "prague": "cs", "lithuania": "lt", "latvia": "lv",
	"estonia": "et", "iceland": "is", "icelandic": "is", "malta": "mt", "maltese": "mt",
	"albania": "sq", "albanian": "sq", "macedonia": "mk", "belarus": "be", "belarusian": "be",
	// ── East / Southeast / Central Asia ──
	"china": "zh-CN", "chinese": "zh-CN", "mandarin": "zh-CN", "beijing": "zh-CN",
	"shanghai": "zh-CN", "taiwan": "zh-TW", "hong kong": "zh-TW",
	"japan": "ja", "japanese": "ja", "tokyo": "ja", "osaka": "ja",
	"korea": "ko", "korean": "ko", "seoul": "ko", "north korea": "ko",
	"mongolia": "mn", "mongolian": "mn", "ul": "mn",
	"thailand": "th", "thai": "th", "bangkok": "th",
	"vietnam": "vi", "vietnamese": "vi", "hanoi": "vi", "saigon": "vi",
	"cambodia": "km", "khmer": "km", "phnom penh": "km",
	"laos": "lo", "lao": "lo", "vientiane": "lo",
	"myanmar": "my", "burmese": "my", "yangon": "my",
	"malaysia": "ms", "malay": "ms", "kuala lumpur": "ms",
	"indonesia": "id", "indonesian": "id", "jakarta": "id", "javanese": "jv",
	"sundanese": "su", "philippines": "fil", "filipino": "fil", "tagalog": "fil",
	"manila": "fil", "cebuano": "ceb", "iloko": "ilo", "singapore": "zh-CN",
	// ── Americas / Oceania ──
	"canada": "en", "toronto": "en", "australia": "en", "sydney": "en",
	"new zealand": "en", "hawaii": "haw", "hawaiian": "haw",
	"peru": "es", "lima": "es", "chile": "es", "santiago": "es", "ecuador": "es",
	"guatemala": "es", "cuba": "es", "havana": "es", "bolivia": "es", "paraguay": "gn",
	"guyana": "en", "suriname": "nl", "haiti": "ht", "haitian creole": "ht",
	"jamaica": "en", "trinidad": "en", "quebec": "fr", "montreal": "fr",
	// ── Native-script language names (users type their language in its own script) ──
	// Punjabi appears in both scripts, and each maps to its own code: Shahmukhi
	// (پنجابی, Pakistan) -> pa-Arab, Gurmukhi (ਪੰਜਾਬੀ, India) -> pa.
	"اردو": "ur", "हिन्दी": "hi", "हिंदी": "hi", "پنجابی": "pa-Arab", "ਪੰਜਾਬੀ": "pa",
	"سرائیکی": "pa-Arab", "ہندکو": "pa-Arab", "مہاجر": "ur",
	"سنڌي": "sd", "سندھی": "sd", "پشتو": "ps", "بلوچی": "bal",
	"العربية": "ar", "فارسی": "fa", "دری": "fa-AF", "کوردی": "ku",
	"中文": "zh-CN", "中国": "zh-CN", "日本語": "ja", "한국어": "ko",
	"Русский": "ru", "Українська": "uk", "Español": "es", "Français": "fr",
	"Deutsch": "de", "Português": "pt", "Italiano": "it", "Türkçe": "tr",
	"বাংলা": "bn", "ગુજરાતી": "gu", "தமிழ்": "ta", "తెలుగు": "te",
	"ಕನ್ನಡ": "kn", "മലയാളം": "ml", "मराठी": "mr", "ଓଡ଼ିଆ": "or",
	"অসমীয়া": "as", "සිංහල": "si", "नेपाली": "ne", "امہارک": "am",
	"አማርኛ": "am", "Kiswahili": "sw", "ةيبرعلا": "ar",
}

// trtLangIndex maps a lower-cased code OR name to the canonical code.
var trtLangIndex = func() map[string]string {
	m := make(map[string]string, len(trtLangs)*2+len(trtRegionAliases))
	for _, l := range trtLangs {
		m[strings.ToLower(l.Code)] = l.Code
		m[strings.ToLower(l.Name)] = l.Code
	}
	for name, code := range trtRegionAliases {
		m[strings.ToLower(name)] = code
	}
	// common aliases
	m["chinese"] = "zh-CN"
	m["mandarin"] = "zh-CN"
	m["tagalog"] = "fil"
	m["farsi"] = "fa"
	m["dari"] = "fa-AF"
	m["punjabi (pakistan)"] = "pa-Arab"
	m["pakistani punjabi"] = "pa-Arab"
	m["shahmukhi"] = "pa-Arab"
	m["saraiki"] = "pa-Arab"
	m["hindko"] = "pa-Arab"
	m["muhajir"] = "ur"
	m["roman urdu"] = "ur"
	m["roman hindi"] = "hi"
	m["roman punjabi"] = "pa"
	m["urdu roman"] = "ur"
	m["hindi roman"] = "hi"
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
	b.WriteString("*TRANSLATE ANY TEXT INTO 250+ LANGUAGES INSTANTLY*\n\n")
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

// trtTranslate translates text into target. Google's free web endpoint is tried
// first; when it rate-limits or errors, MyMemory (also key-less and free) is used
// as a fallback so replies keep getting translated instead of silently reverting
// to English.
func trtTranslate(ctx context.Context, text, target string) (string, string, error) {
	out, detected, err := trtTranslateGoogle(ctx, text, target)
	if err == nil && trtPlausible(text, out) {
		return out, detected, nil
	}
	// Google's free endpoint throttles heavy callers by returning a degenerate
	// one-character "translation" rather than an HTTP error, so an implausible
	// result is treated as a failure and the key-less fallback takes over.
	if fb, ferr := trtTranslateMyMemory(ctx, text, target); ferr == nil {
		return fb, "", nil
	}
	if err == nil {
		return "", "", fmt.Errorf("implausible translation")
	}
	return "", "", err
}

// trtPlausible rejects the degenerate output the free Google endpoint returns
// when it throttles a caller: a multi-word phrase coming back as one character.
// Without this the throttled reply would silently replace a menu line with "ت".
func trtPlausible(src, out string) bool {
	out = strings.TrimSpace(out)
	if out == "" {
		return false
	}
	if strings.ContainsAny(src, " \t\n") && utf8.RuneCountInString(out) < 3 {
		return false
	}
	return true
}

func trtTranslateGoogle(ctx context.Context, text, target string) (string, string, error) {
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

// trtTranslateMyMemory is the key-less fallback used when Google rate-limits.
// The endpoint caps `q` at 500 bytes, so long text is sent in newline-joined
// chunks and stitched back, preserving the line count callers rely on.
func trtTranslateMyMemory(ctx context.Context, text, target string) (string, error) {
	lines := strings.Split(text, "\n")
	const maxQ = 480
	var out []string
	var chunk []string
	size := 0
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		res, err := trtMyMemoryCall(ctx, strings.Join(chunk, "\n"), target)
		if err != nil {
			return err
		}
		got := strings.Split(res, "\n")
		if len(got) != len(chunk) {
			return fmt.Errorf("mymemory line drift")
		}
		out = append(out, got...)
		chunk, size = nil, 0
		return nil
	}
	for _, ln := range lines {
		if size+len(ln)+1 > maxQ {
			if err := flush(); err != nil {
				return "", err
			}
		}
		chunk = append(chunk, ln)
		size += len(ln) + 1
	}
	if err := flush(); err != nil {
		return "", err
	}
	return strings.Join(out, "\n"), nil
}

func trtMyMemoryCall(ctx context.Context, text, target string) (string, error) {
	u := "https://api.mymemory.translated.net/get?langpair=en|" + url.QueryEscape(target) +
		"&q=" + url.QueryEscape(text)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("mymemory http %d", resp.StatusCode)
	}
	var parsed struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("mymemory parse error")
	}
	res := strings.TrimSpace(parsed.ResponseData.TranslatedText)
	if res == "" {
		return "", fmt.Errorf("empty translation")
	}
	return res, nil
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
	Register(Command{Name: "trt", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO TRANSLATE ANY TEXT INTO 250+ LANGUAGES USING GOOGLE TRANSLATE. USE IT AS .TRT <LANG> <TEXT> OR REPLY TO A MESSAGE WITH .TRT <LANG>.", Run: handleTRT})

	// aliases (Hidden)
	Register(Command{Name: "translate", Hidden: true, Run: handleTRT})
	Register(Command{Name: "tr", Hidden: true, Run: handleTRT})
	Register(Command{Name: "gtrans", Hidden: true, Run: handleTRT})
}
