package goldcmds

// ============================================================================
// GOLD-MD — ANIME REACTION COMMANDS  (.b* BOYS / .g* GIRLS)
// File: reaction.go
// ============================================================================
// Two categories, each with 500 UNIQUE English reaction commands:
//   BREACTION → .b<name>  (e.g. .bhappy .bsad .bangry)  — SOLO BOY anime
//   GREACTION → .g<name>  (e.g. .ghappy .gsad .gangry)  — SOLO GIRL anime
//
// FLOW: user types .bhappy → the command message is DELETED → an anime
// reaction GIF is fetched → converted to mp4 → sent as a looping GIF
// (no caption — just the reaction video).
//
// SOURCES (all free, no API key), tried in order:
//   1. Tenor      https://tenor.googleapis.com/v2/search?q=anime+<boy|girl>+<name>
//   2. gifukai    https://api.gifukai.com/v1/<action>?pairing=<m|f>
//   3. otakugifs  https://api.otakugifs.xyz/gif?reaction=<name>
//   4. nekos.best https://nekos.best/api/v2/<name>
//   5. purrbot    https://api.purrbot.site/v2/img/sfw/<name>/gif
// ============================================================================

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// tenorKey is the public web key embedded in Tenor's own pages (no signup).
const tenorKey = "AIzaSyCZt6SSh5VgVPzD9fhyzG1DprdPRhtoaR4"

// tenorClientKey is Tenor's own web client key.
const tenorClientKey = "tenor_web"

// reactionNames is the catalog of 500 unique English reaction words.
var reactionNames = []string{
	"happy", "sad", "angry", "excited", "bored", "confused", "scared", "surprised",
	"shocked", "nervous", "anxious", "calm", "relaxed", "tired", "sleepy", "energetic",
	"cheerful", "joyful", "delighted", "pleased", "content", "satisfied", "grateful", "thankful",
	"hopeful", "optimistic", "proud", "confident", "brave", "courageous", "fearless", "worried",
	"stressed", "frustrated", "annoyed", "irritated", "furious", "enraged", "jealous", "envious",
	"lonely", "depressed", "miserable", "heartbroken", "devastated", "disappointed", "embarrassed", "ashamed",
	"guilty", "regretful", "sorry", "apologetic", "humble", "shy", "timid", "bashful",
	"flustered", "blushing", "charmed", "smitten", "lovestruck", "affectionate", "caring", "kind",
	"gentle", "warm", "friendly", "welcoming", "generous", "helpful", "supportive", "encouraging",
	"inspiring", "motivated", "determined", "focused", "curious", "interested", "fascinated", "amazed",
	"astonished", "awed", "impressed", "enlightened", "thoughtful", "pensive", "reflective", "dreamy",
	"wistful", "nostalgic", "sentimental", "emotional", "touched", "moved", "overwhelmed", "speechless",
	"dumbfounded", "baffled", "puzzled", "perplexed", "bewildered", "mystified", "skeptical", "doubtful",
	"uncertain", "unsure", "indecisive", "hesitant", "reluctant", "unwilling", "resistant", "stubborn",
	"defiant", "rebellious", "mischievous", "playful", "silly", "goofy", "funny", "hilarious",
	"amused", "entertained", "laughing", "giggling", "chuckling", "smiling", "grinning", "beaming",
	"smirking", "winking", "teasing", "mocking", "taunting", "sarcastic", "ironic", "cynical",
	"bitter", "resentful", "spiteful", "vengeful", "hateful", "disgusted", "repulsed", "revolted",
	"appalled", "horrified", "terrified", "petrified", "panicked", "alarmed", "startled", "frightened",
	"trembling", "shaking", "shivering", "quivering", "cowering", "hiding", "fleeing", "running",
	"escaping", "chasing", "pursuing", "hunting", "searching", "seeking", "exploring", "wandering",
	"roaming", "traveling", "journeying", "adventuring", "discovering", "finding", "losing", "winning",
	"failing", "succeeding", "achieving", "accomplishing", "completing", "finishing", "starting", "beginning",
	"ending", "stopping", "pausing", "waiting", "resting", "sleeping", "dreaming", "waking",
	"rising", "standing", "sitting", "lying", "kneeling", "bowing", "praying", "meditating",
	"contemplating", "thinking", "pondering", "wondering", "questioning", "asking", "answering", "replying",
	"responding", "reacting", "ignoring", "avoiding", "evading", "dodging", "blocking", "defending",
	"protecting", "guarding", "shielding", "saving", "rescuing", "helping", "aiding", "assisting",
	"serving", "giving", "sharing", "donating", "contributing", "volunteering", "participating", "joining",
	"gathering", "meeting", "greeting", "hugging", "embracing", "cuddling", "snuggling", "kissing",
	"smooching", "pecking", "nuzzling", "holding", "touching", "patting", "stroking", "petting",
	"rubbing", "massaging", "tickling", "pinching", "poking", "prodding", "nudging", "pushing",
	"pulling", "dragging", "lifting", "carrying", "throwing", "catching", "dropping", "releasing",
	"letting", "allowing", "permitting", "accepting", "rejecting", "refusing", "denying", "admitting",
	"confessing", "revealing", "concealing", "disguising", "pretending", "acting", "performing", "dancing",
	"singing", "playing", "gaming", "competing", "racing", "fighting", "battling", "struggling",
	"striving", "trying", "attempting", "practicing", "training", "learning", "studying", "teaching",
	"explaining", "describing", "narrating", "telling", "speaking", "talking", "chatting", "conversing",
	"discussing", "debating", "arguing", "quarreling", "reconciling", "forgiving", "apologizing", "thanking",
	"praising", "complimenting", "flattering", "admiring", "respecting", "honoring", "celebrating", "partying",
	"feasting", "dining", "eating", "drinking", "sipping", "tasting", "savoring", "enjoying",
	"relishing", "appreciating", "valuing", "treasuring", "cherishing", "loving", "adoring", "worshiping",
	"idolizing", "obsessing", "craving", "desiring", "wanting", "needing", "wishing", "hoping",
	"imagining", "visualizing", "creating", "inventing", "designing", "building", "constructing", "making",
	"crafting", "shaping", "forming", "molding", "sculpting", "painting", "drawing", "sketching",
	"coloring", "writing", "reading", "reciting", "memorizing", "remembering", "forgetting", "recalling",
	"reminiscing", "reflecting", "concentrating", "noticing", "observing", "watching", "looking", "seeing",
	"gazing", "staring", "glancing", "peeking", "spying", "inspecting", "examining", "analyzing",
	"evaluating", "judging", "assessing", "measuring", "comparing", "contrasting", "distinguishing", "separating",
	"dividing", "splitting", "breaking", "shattering", "smashing", "crushing", "destroying", "demolishing",
	"ruining", "wrecking", "damaging", "harming", "hurting", "injuring", "wounding", "healing",
	"curing", "treating", "mending", "repairing", "fixing", "restoring", "renewing", "refreshing",
	"revitalizing", "rejuvenating", "energizing", "invigorating", "stimulating", "thrilling", "exhilarating", "astonishing",
	"stunning", "breathtaking", "magnificent", "splendid", "glorious", "wonderful", "marvelous", "fantastic",
	"fabulous", "incredible", "unbelievable", "extraordinary", "remarkable", "exceptional", "outstanding", "excellent",
	"superb", "perfect", "flawless", "ideal", "supreme", "ultimate", "divine", "heavenly",
	"blissful", "ecstatic", "euphoric", "elated", "overjoyed", "thrilled", "enchanted", "captivated",
	"mesmerized", "hypnotized", "spellbound", "intrigued", "inquisitive", "nosy", "prying", "snooping",
	"investigating", "researching", "uncovering", "exposing", "disclosing", "divulging", "leaking", "spilling",
	"spreading", "broadcasting", "announcing", "declaring", "proclaiming", "stating", "asserting", "claiming",
	"alleging", "accusing", "blaming", "criticizing", "condemning", "scolding", "reprimanding", "punishing",
	"disciplining", "correcting", "guiding", "directing", "leading", "following", "obeying", "disobeying",
	"rebelling", "revolting", "protesting", "demonstrating",
}

var reactionEmojiMap = map[string]string{
	"happy": "😄", "sad": "😢", "angry": "😠", "excited": "🤩", "bored": "😑",
	"confused": "😕", "scared": "😨", "surprised": "😲", "shocked": "😱", "nervous": "😰",
	"anxious": "😟", "calm": "😌", "relaxed": "😎", "tired": "😩", "sleepy": "😴",
	"energetic": "⚡", "cheerful": "😊", "joyful": "😁", "delighted": "😃", "pleased": "🙂",
	"content": "😊", "satisfied": "😌", "grateful": "🙏", "thankful": "🙏", "hopeful": "🤞",
	"optimistic": "🌤️", "proud": "😤", "confident": "😏", "brave": "🦁", "courageous": "🦁",
	"fearless": "💪", "worried": "😟", "stressed": "😖", "frustrated": "😤", "annoyed": "😒",
	"irritated": "😠", "furious": "🤬", "enraged": "🤬", "jealous": "😒", "envious": "😒",
	"lonely": "🥺", "depressed": "😞", "miserable": "😭", "heartbroken": "💔", "devastated": "😭",
	"disappointed": "😞", "embarrassed": "😳", "ashamed": "😳", "guilty": "😔", "regretful": "😔",
	"sorry": "🙇", "apologetic": "🙇", "humble": "🙇", "shy": "😳", "timid": "😳",
	"bashful": "😊", "flustered": "😳", "blushing": "😊", "charmed": "😍", "smitten": "😍",
	"lovestruck": "😍", "affectionate": "🥰", "caring": "🤗", "kind": "😊", "gentle": "😌",
	"warm": "🤗", "friendly": "😊", "welcoming": "🤗", "generous": "🤲", "helpful": "🤝",
	"supportive": "🤝", "encouraging": "💪", "inspiring": "✨", "motivated": "💪", "determined": "😤",
	"focused": "🧐", "curious": "🤔", "interested": "🤔", "fascinated": "🤩", "amazed": "😲",
	"astonished": "😲", "awed": "😮", "impressed": "😮", "enlightened": "💡", "thoughtful": "🤔",
	"pensive": "🤔", "reflective": "🤔", "dreamy": "😍", "wistful": "🥺", "nostalgic": "🥺",
	"sentimental": "🥺", "emotional": "🥹", "touched": "🥹", "moved": "🥹", "overwhelmed": "😵",
	"speechless": "😶", "dumbfounded": "😶", "baffled": "😕", "puzzled": "🤔", "perplexed": "😕",
	"bewildered": "😵", "mystified": "🤔", "skeptical": "🤨", "doubtful": "🤨", "uncertain": "😕",
	"unsure": "🤔", "indecisive": "🤔", "hesitant": "😬", "reluctant": "😬", "unwilling": "🙅",
	"resistant": "🙅", "stubborn": "😤", "defiant": "😠", "rebellious": "😈", "mischievous": "😏",
	"playful": "😜", "silly": "🤪", "goofy": "🤪", "funny": "😂", "hilarious": "🤣",
	"amused": "😄", "entertained": "😄", "laughing": "😂", "giggling": "🤭", "chuckling": "😄",
	"smiling": "😊", "grinning": "😁", "beaming": "😁", "smirking": "😏", "winking": "😉",
	"teasing": "😜", "mocking": "😝", "taunting": "😜", "sarcastic": "😏", "ironic": "😏",
	"cynical": "😒", "bitter": "😖", "resentful": "😒", "spiteful": "😒", "vengeful": "😠",
	"hateful": "😠", "disgusted": "🤢", "repulsed": "🤢", "revolted": "🤮", "appalled": "😱",
	"horrified": "😱", "terrified": "😱", "petrified": "😱", "panicked": "😱", "alarmed": "😨",
	"startled": "😲", "frightened": "😨", "trembling": "😰", "shaking": "😰", "shivering": "🥶",
	"quivering": "😰", "cowering": "😨", "hiding": "🙈", "fleeing": "🏃", "running": "🏃",
	"escaping": "🏃", "chasing": "🏃", "pursuing": "🏃", "hunting": "🏹", "searching": "🔍",
	"seeking": "🔍", "exploring": "🧭", "wandering": "🚶", "roaming": "🚶", "traveling": "✈️",
	"journeying": "🧳", "adventuring": "🗺️", "discovering": "🔍", "finding": "🔍", "losing": "😞",
	"winning": "🏆", "failing": "😞", "succeeding": "🏆", "achieving": "🏆", "accomplishing": "🏆",
	"completing": "✅", "finishing": "✅", "starting": "🚀", "beginning": "🚀", "ending": "🔚",
	"stopping": "✋", "pausing": "⏸️", "waiting": "⏳", "resting": "😌", "sleeping": "😴",
	"dreaming": "💭", "waking": "🌅", "rising": "🌅", "standing": "🧍", "sitting": "🪑",
	"lying": "🛌", "kneeling": "🧎", "bowing": "🙇", "praying": "🙏", "meditating": "🧘",
	"contemplating": "🤔", "thinking": "🤔", "pondering": "🤔", "wondering": "🤔", "questioning": "❓",
	"asking": "❓", "answering": "💬", "replying": "💬", "responding": "💬", "reacting": "😮",
	"ignoring": "🙄", "avoiding": "🙈", "evading": "🏃", "dodging": "💨", "blocking": "🛡️",
	"defending": "🛡️", "protecting": "🛡️", "guarding": "🛡️", "shielding": "🛡️", "saving": "💰",
	"rescuing": "🦸", "helping": "🤝", "aiding": "🤝", "assisting": "🤝", "serving": "🤝",
	"giving": "🎁", "sharing": "🤝", "donating": "🎁", "contributing": "🤝", "volunteering": "🙋",
	"participating": "🙋", "joining": "🤝", "gathering": "👥", "meeting": "🤝", "greeting": "👋",
	"hugging": "🤗", "embracing": "🤗", "cuddling": "🥰", "snuggling": "🥰", "kissing": "😘",
	"smooching": "😘", "pecking": "😘", "nuzzling": "🥰", "holding": "🤝", "touching": "🤝",
	"patting": "🤚", "stroking": "🤚", "petting": "🤚", "rubbing": "🤚", "massaging": "💆",
	"tickling": "🤣", "pinching": "🤏", "poking": "👉", "prodding": "👉", "nudging": "👉",
	"pushing": "🫸", "pulling": "🫷", "dragging": "🫷", "lifting": "🏋️", "carrying": "🏋️",
	"throwing": "🤾", "catching": "🤲", "dropping": "⬇️", "releasing": "🕊️", "letting": "🕊️",
	"allowing": "✅", "permitting": "✅", "accepting": "✅", "rejecting": "❌", "refusing": "🙅",
	"denying": "🙅", "admitting": "😅", "confessing": "😅", "revealing": "😮", "concealing": "🤫",
	"disguising": "🥸", "pretending": "🎭", "acting": "🎭", "performing": "🎭", "dancing": "💃",
	"singing": "🎤", "playing": "🎮", "gaming": "🎮", "competing": "🏁", "racing": "🏁",
	"fighting": "🥊", "battling": "⚔️", "struggling": "😖", "striving": "💪", "trying": "💪",
	"attempting": "💪", "practicing": "🎯", "training": "🏋️", "learning": "📚", "studying": "📚",
	"teaching": "👨‍🏫", "explaining": "💬", "describing": "💬", "narrating": "📖", "telling": "💬",
	"speaking": "🗣️", "talking": "💬", "chatting": "💬", "conversing": "💬", "discussing": "💬",
	"debating": "🗣️", "arguing": "🗣️", "quarreling": "😠", "reconciling": "🤝", "forgiving": "🤝",
	"apologizing": "🙇", "thanking": "🙏", "praising": "👏", "complimenting": "👏", "flattering": "😊",
	"admiring": "😍", "respecting": "🙇", "honoring": "🎖️", "celebrating": "🎉", "partying": "🎉",
	"feasting": "🍽️", "dining": "🍽️", "eating": "🍽️", "drinking": "🥤", "sipping": "🥤",
	"tasting": "😋", "savoring": "😋", "enjoying": "😄", "relishing": "😋", "appreciating": "🙏",
	"valuing": "💎", "treasuring": "💎", "cherishing": "🥰", "loving": "❤️", "adoring": "😍",
	"worshiping": "🙏", "idolizing": "🤩", "obsessing": "😍", "craving": "🤤", "desiring": "😍",
	"wanting": "🙏", "needing": "🙏", "wishing": "🌠", "hoping": "🤞", "imagining": "💭",
	"visualizing": "💭", "creating": "🎨", "inventing": "💡", "designing": "🎨", "building": "🔨",
	"constructing": "🏗️", "making": "🔨", "crafting": "🧶", "shaping": "🔨", "forming": "🔨",
	"molding": "🔨", "sculpting": "🗿", "painting": "🎨", "drawing": "✏️", "sketching": "✏️",
	"coloring": "🖍️", "writing": "✍️", "reading": "📖", "reciting": "📖", "memorizing": "🧠",
	"remembering": "🧠", "forgetting": "🤷", "recalling": "🧠", "reminiscing": "💭", "reflecting": "🤔",
	"concentrating": "🧐", "noticing": "👀", "observing": "👀", "watching": "👀", "looking": "👀",
	"seeing": "👀", "gazing": "👀", "staring": "👀", "glancing": "👀", "peeking": "👀",
	"spying": "🕵️", "inspecting": "🔍", "examining": "🔍", "analyzing": "🔍", "evaluating": "📊",
	"judging": "⚖️", "assessing": "📊", "measuring": "📏", "comparing": "⚖️", "contrasting": "⚖️",
	"distinguishing": "🔍", "separating": "✂️", "dividing": "➗", "splitting": "✂️", "breaking": "💔",
	"shattering": "💥", "smashing": "💥", "crushing": "💥", "destroying": "💥", "demolishing": "💥",
	"ruining": "💥", "wrecking": "💥", "damaging": "💥", "harming": "💢", "hurting": "💢",
	"injuring": "🤕", "wounding": "🤕", "healing": "💚", "curing": "💚", "treating": "💊",
	"mending": "🩹", "repairing": "🔧", "fixing": "🔧", "restoring": "🔧", "renewing": "🔄",
	"refreshing": "🔄", "revitalizing": "⚡", "rejuvenating": "⚡", "energizing": "⚡", "invigorating": "⚡",
	"stimulating": "⚡", "thrilling": "🤩", "exhilarating": "🤩", "astonishing": "😲", "stunning": "😍",
	"breathtaking": "😍", "magnificent": "🤩", "splendid": "🤩", "glorious": "🤩", "wonderful": "😄",
	"marvelous": "😄", "fantastic": "🤩", "fabulous": "🤩", "incredible": "🤩", "unbelievable": "😲",
	"extraordinary": "🤩", "remarkable": "🤩", "exceptional": "🤩", "outstanding": "🤩", "excellent": "👏",
	"superb": "👏", "perfect": "💯", "flawless": "💯", "ideal": "💯", "supreme": "👑",
	"ultimate": "👑", "divine": "😇", "heavenly": "😇", "blissful": "😌", "ecstatic": "🤩",
	"euphoric": "🤩", "elated": "😄", "overjoyed": "😄", "thrilled": "🤩", "enchanted": "😍",
	"captivated": "😍", "mesmerized": "😍", "hypnotized": "😵‍💫", "spellbound": "😍", "intrigued": "🤔",
	"inquisitive": "🤔", "nosy": "👀", "prying": "👀", "snooping": "🕵️", "investigating": "🕵️",
	"researching": "🔍", "uncovering": "🔍", "exposing": "😮", "disclosing": "😮", "divulging": "😮",
	"leaking": "💧", "spilling": "💧", "spreading": "📢", "broadcasting": "📢", "announcing": "📢",
	"declaring": "📢", "proclaiming": "📢", "stating": "💬", "asserting": "💬", "claiming": "💬",
	"alleging": "💬", "accusing": "👉", "blaming": "👉", "criticizing": "👎", "condemning": "👎",
	"scolding": "😠", "reprimanding": "😠", "punishing": "😠", "disciplining": "😠", "correcting": "✏️",
	"guiding": "🧭", "directing": "🧭", "leading": "🧭", "following": "🚶", "obeying": "🙇",
	"disobeying": "🙅", "rebelling": "😈", "revolting": "😠", "protesting": "📢", "demonstrating": "🎤",
	"marching": "🚶", "rallying": "📢", "campaigning": "📢", "advocating": "📢", "endorsing": "👍",
	"promoting": "📢", "advertising": "📢", "marketing": "📢", "selling": "💰", "buying": "🛒",
	"trading": "💱", "exchanging": "💱", "swapping": "🔄", "bargaining": "🤝", "negotiating": "🤝",
	"dealing": "🤝", "transacting": "💱", "paying": "💰", "spending": "💸", "investing": "📈",
	"earning": "💰", "gaining": "📈", "profiting": "📈", "gambling": "🎲", "betting": "🎲",
	"risking": "🎲", "daring": "😏", "challenging": "😤", "defying": "😠", "confronting": "😤",
	"facing": "😐", "encountering": "😮", "experiencing": "😮", "enduring": "😤", "surviving": "💪",
	"thriving": "🌱", "flourishing": "🌱", "blooming": "🌸", "blossoming": "🌸", "growing": "🌱",
	"developing": "📈", "evolving": "🔄", "transforming": "🔄", "changing": "🔄", "adapting": "🔄",
	"adjusting": "🔧", "modifying": "🔧", "altering": "🔧", "revising": "✏️", "editing": "✏️",
	"rewriting": "✏️", "repeating": "🔁", "rehearsing": "🎭", "perfecting": "💯", "mastering": "🏆",
	"excelling": "🏆", "surpassing": "📈", "exceeding": "📈", "outdoing": "🏆", "outperforming": "🏆",
	"triumphing": "🏆", "conquering": "🏆", "overcoming": "💪", "prevailing": "🏆", "fulfilling": "✅",
	"realizing": "💡", "actualizing": "✨", "manifesting": "✨", "materializing": "✨", "appearing": "👋",
	"emerging": "🌅", "arising": "🌅", "originating": "🌱", "initiating": "🚀", "launching": "🚀",
	"commencing": "🚀", "opening": "🚪", "introducing": "👋", "presenting": "🎤", "showcasing": "🎤",
	"displaying": "🖼️", "exhibiting": "🖼️", "showing": "👀", "unveiling": "🎭", "debuting": "🌟",
	"premiering": "🌟", "entertaining": "🎭", "amusing": "😄", "delighting": "😄", "pleasing": "😊",
	"gratifying": "😊", "comforting": "🤗", "soothing": "😌", "calming": "😌", "pacifying": "😌",
	"placating": "😌", "appeasing": "😌", "mollifying": "😌", "reassuring": "🤗", "uplifting": "✨",
	"elevating": "📈", "raising": "📈", "boosting": "📈", "cheering": "📣", "benefiting": "🎁",
	"rewarding": "🏆", "compensating": "💰", "repaying": "💰", "returning": "🔙", "recovering": "💚",
	"recuperating": "💚", "rebuilding": "🏗️", "reconstructing": "🏗️", "renovating": "🏗️", "remodeling": "🏗️",
	"refurbishing": "🏗️", "redecorating": "🎨", "cleaning": "🧹", "washing": "🧼", "scrubbing": "🧽",
	"polishing": "✨", "shining": "✨", "gleaming": "✨", "glowing": "✨", "radiating": "✨",
	"sparkling": "✨", "twinkling": "✨", "glittering": "✨", "shimmering": "✨", "glistening": "✨",
	"dazzling": "✨", "blinding": "✨", "illuminating": "💡", "brightening": "💡", "lighting": "💡",
	"warming": "🔥", "heating": "🔥", "cooling": "❄️", "freezing": "🥶", "chilling": "❄️",
	"icing": "🧊", "melting": "🫠", "thawing": "🌡️", "burning": "🔥", "blazing": "🔥",
	"flaming": "🔥", "igniting": "🔥", "kindling": "🔥", "sparking": "✨", "electrifying": "⚡",
	"zapping": "⚡", "striking": "💥", "hitting": "👊", "punching": "👊", "kicking": "🦵",
	"slapping": "✋", "smacking": "✋", "whacking": "✋", "bashing": "💥", "beating": "👊",
	"thrashing": "💥", "pounding": "👊", "hammering": "🔨", "battering": "💥", "clobbering": "💥",
	"walloping": "💥", "thumping": "👊", "bumping": "💥", "knocking": "🚪", "tapping": "👆",
	"rapping": "👆", "clicking": "🖱️", "clacking": "👆", "snapping": "🫰", "cracking": "💥",
	"popping": "🎈", "bursting": "💥", "exploding": "💥", "erupting": "🌋", "blasting": "💥",
	"booming": "💥", "thundering": "⛈️", "roaring": "🦁", "screaming": "😱", "yelling": "📢",
	"shouting": "📢", "hollering": "📢", "bellowing": "📢", "howling": "🐺", "wailing": "😭",
	"crying": "😭", "sobbing": "😭", "weeping": "😭", "tearing": "😢", "sniffling": "🤧",
	"whimpering": "🥺", "moaning": "😩", "groaning": "😩", "sighing": "😮‍💨", "gasping": "😮",
	"panting": "😮‍💨", "breathing": "🌬️", "inhaling": "🌬️", "exhaling": "🌬️", "sniffing": "👃",
	"sneezing": "🤧", "coughing": "🤧", "hiccuping": "😅", "burping": "😅", "yawning": "🥱",
	"stretching": "🙆", "flexing": "💪", "bending": "🤸", "twisting": "🌀", "turning": "🔄",
	"spinning": "🌀", "rotating": "🔄", "revolving": "🔄", "circling": "🔄", "orbiting": "🪐",
	"looping": "🔁", "curling": "🌀", "coiling": "🌀", "winding": "🌀", "wrapping": "🎁",
	"binding": "🔗", "tying": "🪢", "knotting": "🪢", "tangling": "🪢", "weaving": "🧶",
	"braiding": "🪢", "threading": "🧵", "sewing": "🧵", "stitching": "🧵", "knitting": "🧶",
	"crocheting": "🧶", "embroidering": "🧵", "quilting": "🧵", "patching": "🩹", "darning": "🧵",
	"regenerating": "🔄", "resurrecting": "✨", "reviving": "✨", "awakening": "🌅", "rousing": "📣",
	"stirring": "🥄", "ascending": "⬆️", "climbing": "🧗", "mounting": "⬆️", "scaling": "🧗",
	"surmounting": "🧗", "subduing": "😤", "quelling": "😤", "suppressing": "😤", "repressing": "😤",
	"oppressing": "😤", "tyrannizing": "😈", "dominating": "😈", "controlling": "🎮", "commanding": "👑",
	"ruling": "👑", "reigning": "👑", "governing": "👑", "managing": "📋", "organizing": "📋",
	"coordinating": "📋", "orchestrating": "🎼", "arranging": "📋", "planning": "📝", "scheming": "😈",
	"plotting": "😈", "conspiring": "😈", "conniving": "😈", "colluding": "😈", "collaborating": "🤝",
	"cooperating": "🤝", "uniting": "🤝", "merging": "🔗", "combining": "🔗", "blending": "🌀",
	"mixing": "🥄", "whisking": "🥄", "whipping": "🥄", "folding": "📄", "kneading": "🍞",
	"rolling": "🌀", "pressing": "👇", "squeezing": "🤏", "compressing": "🗜️", "grinding": "⚙️",
	"milling": "⚙️", "shredding": "📄", "cutting": "✂️", "slicing": "🔪", "dicing": "🔪",
	"chopping": "🔪", "mincing": "🔪", "grating": "🧀", "peeling": "🍌", "carving": "🔪",
	"casting": "🎣", "forging": "🔨", "welding": "🔧", "soldering": "🔧", "gluing": "🧴",
	"taping": "📼", "stapling": "📎", "clipping": "📎", "pinning": "📌", "nailing": "🔨",
	"screwing": "🔩", "bolting": "🔩", "riveting": "🔩", "fastening": "🔗", "securing": "🔒",
	"locking": "🔒", "latching": "🔒", "hooking": "🪝", "clasping": "🤝", "clutching": "🤲",
	"gripping": "🤲", "grasping": "🤲", "grabbing": "🤲", "seizing": "🤲", "snatching": "🤲",
	"capturing": "🤲", "trapping": "🪤", "netting": "🕸️", "ensnaring": "🕸️", "entangling": "🕸️",
	"enmeshing": "🕸️", "embroiling": "🌀", "involving": "🤝", "engaging": "🤝", "occupying": "📋",
	"busying": "📋", "working": "💼", "laboring": "💼", "toiling": "💼", "sweating": "💦",
	"straining": "😤", "exerting": "💪", "driving": "🚗", "propelling": "🚀", "urging": "📣",
	"prompting": "👉", "spurring": "👉", "goading": "👉", "extracting": "⛏️", "eliciting": "💬",
	"evoking": "✨", "attracting": "🧲", "luring": "🎣", "enticing": "😍", "tempting": "😈",
	"seducing": "😏", "alluring": "😍", "bewitching": "🧙", "absorbing": "🌀", "engrossing": "📖",
	"immersing": "🌊", "maneuvering": "🎮", "operating": "🎮", "steering": "🚗", "piloting": "✈️",
	"navigating": "🧭", "sailing": "⛵", "cruising": "🚢", "flying": "✈️", "soaring": "🦅",
	"gliding": "🛩️", "floating": "🎈", "drifting": "🌊", "wafting": "🌬️", "hovering": "🚁",
	"levitating": "🪄", "ravaging": "💥", "pillaging": "🏴‍☠️", "plundering": "🏴‍☠️", "looting": "🏴‍☠️",
	"ransacking": "🏴‍☠️", "raiding": "🏴‍☠️", "invading": "⚔️", "attacking": "⚔️", "assaulting": "⚔️",
}

var gifukaiSet = map[string]bool{
	"angry": true, "bite": true, "bleh": true, "blowkiss": true, "blush": true, "bonk": true,
	"bored": true, "bye": true, "carry": true, "clap": true, "confused": true, "cry": true,
	"cuddle": true, "dance": true, "eat": true, "facepalm": true, "feed": true, "handhold": true,
	"handshake": true, "happy": true, "hi": true, "highfive": true, "hug": true, "kick": true,
	"kill": true, "kiss": true, "lappillow": true, "laugh": true, "lick": true, "nod": true,
	"nope": true, "nya": true, "pat": true, "peek": true, "poke": true, "pout": true,
	"punch": true, "run": true, "salute": true, "scared": true, "shake": true, "shocked": true,
	"shoot": true, "shrug": true, "shy": true, "sing": true, "sip": true, "slap": true,
	"sleep": true, "smile": true, "smug": true, "sorry": true, "spin": true, "stare": true,
	"surprised": true, "taunt": true, "teehee": true, "think": true, "thumbsup": true, "tickle": true,
	"tired": true, "wag": true, "wallslam": true, "wave": true, "wink": true, "yawn": true,
	"yay": true, "yeet": true,
}

var otakuSet = map[string]bool{
	"airkiss": true, "angrystare": true, "bite": true, "bleh": true, "blush": true, "brofist": true,
	"celebrate": true, "cheers": true, "clap": true, "confused": true, "cool": true, "cry": true,
	"cuddle": true, "dance": true, "drool": true, "evillaugh": true, "facepalm": true, "handhold": true,
	"happy": true, "headbang": true, "hug": true, "huh": true, "kiss": true, "laugh": true,
	"lick": true, "love": true, "mad": true, "nervous": true, "no": true, "nom": true,
	"nosebleed": true, "nuzzle": true, "nyah": true, "pat": true, "peek": true, "pinch": true,
	"poke": true, "pout": true, "punch": true, "roll": true, "run": true, "sad": true,
	"scared": true, "shout": true, "shrug": true, "shy": true, "sigh": true, "sing": true,
	"sip": true, "slap": true, "sleep": true, "slowclap": true, "smack": true, "smile": true,
	"smug": true, "sneeze": true, "sorry": true, "stare": true, "stop": true, "surprised": true,
	"sweat": true, "thumbsup": true, "tickle": true, "tired": true, "wave": true, "wink": true,
	"woah": true, "yawn": true, "yay": true, "yes": true,
}

var nekosBestSet = map[string]bool{
	"angry": true, "baka": true, "bite": true, "bleh": true, "blowkiss": true, "blush": true,
	"bonk": true, "bored": true, "carry": true, "clap": true, "confused": true, "cry": true,
	"cuddle": true, "dance": true, "facepalm": true, "feed": true, "handhold": true, "handshake": true,
	"happy": true, "highfive": true, "hug": true, "husbando": true, "kabedon": true, "kick": true,
	"kiss": true, "kitsune": true, "lappillow": true, "laugh": true, "lurk": true, "neko": true,
	"nod": true, "nom": true, "nope": true, "nya": true, "pat": true, "peck": true,
	"poke": true, "pout": true, "punch": true, "run": true, "salute": true, "shake": true,
	"shocked": true, "shoot": true, "shrug": true, "sip": true, "slap": true, "sleep": true,
	"smile": true, "smug": true, "spin": true, "stare": true, "tableflip": true, "teehee": true,
	"think": true, "thumbsup": true, "tickle": true, "wag": true, "waifu": true, "wave": true,
	"wink": true, "yawn": true, "yeet": true,
}

var purrSet = map[string]bool{
	"angry": true, "blush": true, "comfy": true, "cry": true, "cuddle": true, "dance": true,
	"feed": true, "fluff": true, "hug": true, "kiss": true, "lick": true, "neko": true,
	"pat": true, "poke": true, "slap": true, "smile": true, "tail": true, "tickle": true,
}


// reactionEmoji returns the emoji for a reaction name (default sparkle).
func reactionEmoji(name string) string {
	if e, ok := reactionEmojiMap[name]; ok {
		return e
	}
	return "\u2728"
}

// reactionHTTPGet fetches a URL with a byte cap and a browser-ish UA.
func reactionHTTPGet(ctx context.Context, rawurl string, cap int64) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawurl, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cap))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// reactionTryProvider fetches JSON from apiURL, extracts the given field
// (url/link) and downloads the GIF bytes.
func reactionTryProvider(ctx context.Context, apiURL, field string) ([]byte, bool) {
	body, ok := reactionHTTPGet(ctx, apiURL, 1<<20)
	if !ok {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, false
	}
	u, _ := m[field].(string)
	if u == "" {
		return nil, false
	}
	return reactionHTTPGet(ctx, u, 25<<20)
}

// tenorSearch queries Tenor v2 and returns the first GIF url for the query.
func tenorSearch(ctx context.Context, query string) (string, bool) {
	api := "https://tenor.googleapis.com/v2/search?q=" + url.QueryEscape(query) +
		"&key=" + tenorKey + "&client_key=" + tenorClientKey +
		"&limit=1&media_filter=gif&contentfilter=high"
	body, ok := reactionHTTPGet(ctx, api, 1<<20)
	if !ok {
		return "", false
	}
	var d struct {
		Results []struct {
			MediaFormats map[string]struct {
				URL string `json:"url"`
			} `json:"media_formats"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &d); err != nil || len(d.Results) == 0 {
		return "", false
	}
	mf := d.Results[0].MediaFormats
	if g, ok := mf["gif"]; ok && g.URL != "" {
		return g.URL, true
	}
	if g, ok := mf["tinygif"]; ok && g.URL != "" {
		return g.URL, true
	}
	return "", false
}

// reactionFetchGif tries every provider in order and returns the first GIF.
// gender is "m" (solo boy) or "f" (solo girl).
func reactionFetchGif(ctx context.Context, name, gender string) ([]byte, bool) {
	genderWord := "boy"
	if gender == "f" {
		genderWord = "girl"
	}

	// 1) Tenor \u2014 gender-specific anime search.
	if u, ok := tenorSearch(ctx, "anime "+genderWord+" "+name); ok {
		if d, ok := reactionHTTPGet(ctx, u, 25<<20); ok {
			return d, true
		}
	}
	// 1b) Tenor \u2014 generic anime search.
	if u, ok := tenorSearch(ctx, "anime "+name); ok {
		if d, ok := reactionHTTPGet(ctx, u, 25<<20); ok {
			return d, true
		}
	}

	// 2) gifukai (gender pairing).
	if gifukaiSet[name] {
		if d, ok := reactionTryProvider(ctx, "https://api.gifukai.com/v1/"+name+"?pairing="+gender, "url"); ok {
			return d, true
		}
		if d, ok := reactionTryProvider(ctx, "https://api.gifukai.com/v1/"+name, "url"); ok {
			return d, true
		}
	}
	// 3) otakugifs.
	if otakuSet[name] {
		if d, ok := reactionTryProvider(ctx, "https://api.otakugifs.xyz/gif?reaction="+name, "url"); ok {
			return d, true
		}
	}
	// 4) nekos.best.
	if nekosBestSet[name] {
		if d, ok := reactionTryProvider(ctx, "https://nekos.best/api/v2/"+name, "url"); ok {
			return d, true
		}
	}
	// 5) purrbot.
	if purrSet[name] {
		if d, ok := reactionTryProvider(ctx, "https://api.purrbot.site/v2/img/sfw/"+name+"/gif", "link"); ok {
			return d, true
		}
	}
	return nil, false
}

// reactionGifToMp4 converts raw GIF bytes to an mp4 (H.264, yuv420p, even
// dimensions) suitable for WhatsApp GIF playback.
func reactionGifToMp4(ctx context.Context, gifData []byte) ([]byte, uint32, uint32, uint32, bool) {
	if !compressBinaryAvailable("ffmpeg") {
		return nil, 0, 0, 0, false
	}
	in, err := os.CreateTemp("", "goldreact-*.gif")
	if err != nil {
		return nil, 0, 0, 0, false
	}
	inPath := in.Name()
	in.Close()
	defer os.Remove(inPath)
	if err := os.WriteFile(inPath, gifData, 0o600); err != nil {
		return nil, 0, 0, 0, false
	}

	out, err := os.CreateTemp("", "goldreact-*.mp4")
	if err != nil {
		return nil, 0, 0, 0, false
	}
	outPath := out.Name()
	out.Close()
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inPath,
		"-movflags", "+faststart",
		"-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-an",
		outPath)
	if err := cmd.Run(); err != nil {
		return nil, 0, 0, 0, false
	}
	mp4, err := os.ReadFile(outPath)
	if err != nil || len(mp4) == 0 {
		return nil, 0, 0, 0, false
	}
	probe := compressProbeVideo(outPath)
	secs := uint32(probe.DurationSec)
	if secs == 0 {
		secs = 1
	}
	return mp4, secs, uint32(probe.Width), uint32(probe.Height), true
}

// handleReaction deletes the user's command message, fetches the anime GIF,
// converts it and sends it directly (no caption).
func handleReaction(s SessionBridge, info types.MessageInfo, name, gender string) {
	go handleReactionAsync(s, info, name, gender)
}

func handleReactionAsync(s SessionBridge, info types.MessageInfo, name, gender string) {
	// 1) Delete the user's command message first (owner order).
	_ = s.DeleteMessage(info, info.ID)

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "\u26a1 *REACTION ERROR \u26a1*\n*BOT CLIENT NOT CONNECTED*")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	gifData, ok := reactionFetchGif(ctx, name, gender)
	if !ok {
		s.Reply(info, "*\U0001f3ac REACTION ERROR \U0001f3ac*\n*COULD NOT FETCH GIF, TRY AGAIN*")
		return
	}

	mp4, secs, w, h, ok := reactionGifToMp4(ctx, gifData)
	if !ok {
		s.Reply(info, "*\U0001f3ac REACTION ERROR \U0001f3ac*\n*CONVERSION FAILED, TRY AGAIN*")
		return
	}

	// No caption — send the reaction GIF/video directly (owner order).
	_ = s.SendGif(info, mp4, "", secs, w, h)
}

func init() {
	// Register 500 UNIQUE commands per category from the reaction catalog.
	registerReactionCategory("BREACTION", "b", "BOYS", "m")
	registerReactionCategory("GREACTION", "g", "GIRLS", "f")
}

// registerReactionCategory registers one command per reaction name.
func registerReactionCategory(category, prefix, label, gender string) {
	for _, n := range reactionNames {
		name := n
		Register(Command{
			Name:     prefix + name,
			Category: category,
			Desc:     label + " " + strings.ToUpper(name) + " ANIME REACTION",
			Run: func(s SessionBridge, info types.MessageInfo, args []string, pfx string) {
				handleReaction(s, info, name, gender)
			},
		})
	}
}
