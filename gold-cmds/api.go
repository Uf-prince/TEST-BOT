package goldcmds

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
)

// DefaultBotNameMarker is a sentinel value stored in the Redis "botname"
// field to indicate "use the branded GOLD-MD default footer". It is set
// automatically on fresh pair (PairSuccess) and when the owner runs
// .botname reset. When the owner sets a custom name via .botname <name>,
// the real name replaces the marker. Using a marker (instead of an empty
// string) means the field always has an explicit value, matching the
// pattern of other configs (antidelete, prefix, etc.).
const DefaultBotNameMarker = "\x01GOLD_MD_DEFAULT_FOOTER\x01"

// VideoResult represents a single YouTube search result.
type VideoResult struct {
	Title     string
	URL       string
	Thumbnail string
	Duration  string
}

// SessionBridge is the interface that the main package implements to give
// the gold-cmds plugin package access to WhatsApp client operations.
type SessionBridge interface {
	GetClient() *whatsmeow.Client
	GetJID() string
	// SetCmdContext stores the running command's watchdog context on the
	// bridge so the guard compressor (ffmpeg) is killed the moment the
	// command timeout fires — the compressor is now part of the SAME
	// 2-min / 40s budget as the command itself (owner order).
	SetCmdContext(ctx context.Context)
	// BeginGuard starts a PER-SESSION / PER-USER "latest wins" guard for the
	// current command. It cancels any previous in-flight command of the SAME
	// user on the SAME session (so a newer request instantly aborts the old
	// download / ffmpeg work), and returns a cancellable context + a release
	// func the caller MUST defer. Commands of OTHER users (same session) and
	// of ANY user on OTHER sessions are never affected — each session has
	// its own independent guard.
	BeginGuard(userJID string) (context.Context, func())
	Reply(info types.MessageInfo, text string)
	ReplyWithID(info types.MessageInfo, text string) string
	// ReplyWithMentions sends a text message that pings the supplied JIDs
	// (ContextInfo.MentionedJID). Used by anti-* warn/kick notifications.
	ReplyWithMentions(info types.MessageInfo, text string, mentioned []string)
	// SendContact sends a WhatsApp Contact card (vCard) to the chat.
	// displayName is the name shown on the contact card, number is the
	// phone number (digits only, international format without +).
	SendContact(info types.MessageInfo, displayName string, number string) error
	EditMessage(info types.MessageInfo, messageID string, newText string) bool
	DeleteMessage(info types.MessageInfo, messageID string) error
	SendVideo(info types.MessageInfo, data []byte, caption string, thumbnail []byte, seconds uint32, width uint32, height uint32) error
	SendVideoFile(info types.MessageInfo, path string, caption string, thumbnail []byte, seconds uint32, width uint32, height uint32) error
	// SendVideoFileRaw sends a video with NO caption / NO thumbnail / NO
	// footer (raw). Used by .video3. Still runs the guard compressor.
	SendVideoFileRaw(info types.MessageInfo, path string) error
	SendAudio(info types.MessageInfo, data []byte, caption string, seconds uint32) error
	SendAudioFile(info types.MessageInfo, path string, caption string, seconds uint32) error
	// SendAudioFileRaw sends audio with NO caption / NO footer (raw). Used by
	// .play3. Still runs the guard compressor.
	SendAudioFileRaw(info types.MessageInfo, path string) error
	// SendVideoFileRawWait is the .video3 OWNER-ORDERED raw flow: the video is
	// FULLY compressed FIRST (guard), then beforeSend runs (delete waiting msg),
	// and only then the already-compressed RAW video is sent (no caption /
	// thumbnail / footer). Keeps the waiting msg alive until compression ends.
	SendVideoFileRawWait(info types.MessageInfo, path string, beforeSend func()) error
	// SendAudioFileRawWait is the .play3 OWNER-ORDERED raw flow (audio variant).
	SendAudioFileRawWait(info types.MessageInfo, path string, beforeSend func()) error
	// SendVideoThumbFirst is the OWNER-ORDERED flow for .play/.play2/.video/
	// .video2: the video is FULLY compressed FIRST (guard), then the thumbnail
	// + info caption is sent, and only then the already-compressed video. This
	// guarantees the thumbnail never arrives before compression finishes.
	SendVideoThumbFirst(info types.MessageInfo, path string, caption string, thumbnail []byte) error
	// SendAudioThumbFirst is the audio variant of SendVideoThumbFirst.
	SendAudioThumbFirst(info types.MessageInfo, path string, caption string, thumbnail []byte) error
	SendImage(info types.MessageInfo, data []byte, caption string) error
	// SendGif sends an MP4 as a WhatsApp looping GIF (VideoMessage with
	// GifPlayback=true). Used by the .breaction / .greaction anime reaction
	// commands: the source .gif is converted to mp4 by the caller and then
	// sent here so WhatsApp loops it like a GIF.
	SendGif(info types.MessageInfo, data []byte, caption string, seconds uint32, width uint32, height uint32) error
	SetVideoSession(jid string, results []VideoResult)
	SetVideoSession2(jid string, results []VideoResult, hd bool)
	SetVideoSession3(jid string, results []VideoResult, hd bool)
	SetAudioSession(jid string, results []VideoResult)
	SetAudioSession2(jid string, results []VideoResult, play2 bool)
	SetAudioSession3(jid string, results []VideoResult)
	// ClearSearchSession drops any pending search-list pick window for this
	// JID — called whenever a play/video search replaces the pick context.
	ClearSearchSession(jid string)
	// DownloadImage downloads the image attached to the incoming message
	// identified by info. Returns the raw image bytes and true if the message
	// contained an image (direct or quoted); returns nil,false otherwise.
	// Used by the aivideo multi-image flow to collect photos from the user.
	DownloadImage(info types.MessageInfo) ([]byte, bool)
	// DownloadQuotedMedia downloads the media (image/video/audio/sticker/
	// document, including view-once variants) attached to the quoted message
	// of the incoming message identified by info. Returns the raw bytes, the
	// detected mimetype, and true on success; nil,"",false otherwise.
	DownloadQuotedMedia(info types.MessageInfo) ([]byte, string, bool)
	// IsOwner reports whether the sender of info is the bot owner (or the
	// bot's own JID). Owner-only commands gate on this.
	IsOwner(info types.MessageInfo) bool
	// SendDocument sends a generic document (file) message.
	SendDocument(info types.MessageInfo, data []byte, fileName string, mimeType string, caption string) error
	// SendDocumentFile is the streaming variant of SendDocument.
	SendDocumentFile(info types.MessageInfo, path string, fileName string, mimeType string, caption string) error
	// SendSticker sends a WebP sticker (raw bytes).
	SendSticker(info types.MessageInfo, data []byte) error
	// GetMentionedJIDs returns the @-mentioned JIDs parsed from the incoming
	// message identified by info (from ContextInfo.MentionedJID). Returns the
	// list and true if any mentions were present; nil,false otherwise.
	GetMentionedJIDs(info types.MessageInfo) ([]string, bool)
	// RevokeQuotedMessage revokes (deletes for everyone) the quoted/replied-to
	// message. senderJID is the original sender of the quoted message (empty
	// if it was the bot's own message). messageID is the quoted message ID.
	// chat is the chat where the message lives. Returns nil on success.
	RevokeQuotedMessage(chat types.JID, senderJID string, messageID string) error
	// GetPresenceSetting reads a per-user presence setting from Redis
	// (Upstash). field is one of: "alwaysonline", "autotyping",
	// "autorecording". Returns the stored value or def if not set / error.
	GetPresenceSetting(field, def string) string
	// SetPresenceSetting writes a per-user presence setting to Redis.
	SetPresenceSetting(field, val string)
	// SendPresenceUpdate sets the bot's global online/offline presence.
	// "available" = always online, "unavailable" = offline/last seen.
	SendPresenceUpdate(state string) error
	// SendChatPresenceUpdate sends a typing/recording/paused chat presence
	// to a specific chat. state is "composing" or "paused".
	// media is "audio" (recording) or "" (typing text).
	SendChatPresenceUpdate(chat types.JID, state string, media string) error

	// ── STATUS (story) settings ── per-user Redis config ──
	// GetStatusSetting reads a status setting from Redis settings:<botJID>.
	// field is one of: "statusseen", "statusreact", "statusreactemojis",
	// "statusreply", "statusreplymessage". Returns def if not set.
	GetStatusSetting(field, def string) string
	// SetStatusSetting writes a status setting to Redis.
	SetStatusSetting(field, val string)
	// DelStatusSetting removes a status setting from Redis (redis-safe
	// HDEL — no orphan fields). Used by cmdaccess reset.
	DelStatusSetting(field string)

	// ── MODE / BOT PIC per-bot Redis config ──
	// GetModeSetting reads the bot work-mode from Redis settings:<botJID>
	// (field "mode"). Returns def if not set. Values: public/private/groups/inbox.
	GetModeSetting(def string) string
	// SetModeSetting writes the bot work-mode to Redis.
	SetModeSetting(mode string)
	// GetBotPicSetting reads the custom bot menu/alive image URL from Redis
	// (field "botpic"). Returns def ("") if not set.
	GetBotPicSetting(def string) string
	// SetBotPicSetting writes the custom bot menu/alive image URL to Redis.
	SetBotPicSetting(url string)
	// GetBotVideoSetting reads the custom bot menu/alive VIDEO URL from Redis
	// (field "botvideo"). Returns def ("") if not set.
	GetBotVideoSetting(def string) string
	// SetBotVideoSetting writes the custom bot menu/alive VIDEO URL to Redis.
	SetBotVideoSetting(url string)
	// GetBotVoiceSetting reads the custom bot menu/alive VOICE (audio) URL from
	// Redis (field "botvoice"). Returns def ("") if not set.
	GetBotVoiceSetting(def string) string
	// SetBotVoiceSetting writes the custom bot menu/alive VOICE URL to Redis.
	// The special value "off" (goldcmds.MenuMediaVoiceOff) silences the voice.
	SetBotVoiceSetting(url string)
	// VoiceURLPlayable reports whether a voice URL actually serves audio (not
	// an HTML share/error page). Used to reject bad links before storing them.
	VoiceURLPlayable(url string) bool
	// GetMenuStyleSetting reads ONE menu's style override (Redis field
	// "menustyle:<key>"). Returns def ("") when the menu inherits the bot-wide
	// style. Set with ".<x>style SET <n>".
	GetMenuStyleSetting(key, def string) string
	// SetMenuStyleSetting writes ONE menu's style override. Empty clears it.
	SetMenuStyleSetting(key, val string)
	// GetBotMenuStyleSetting reads the bot-wide menu style (field
	// "botmenustyle"). Empty means the built-in classic style.
	GetBotMenuStyleSetting(def string) string
	// SetBotMenuStyleSetting writes the bot-wide menu style. Empty clears it.
	SetBotMenuStyleSetting(val string)
	// GetBotSkinSetting reads the bot-wide TEXT skin (.botstyle) - Redis
	// field "botstyle". A style number 1..50, or "" / "1" for no skin. When
	// set, every outgoing message is rendered in that style's font, marks
	// and symbols (menus included).
	GetBotSkinSetting(def string) string
	// SetBotSkinSetting writes the bot-wide text skin. Empty clears it.
	SetBotSkinSetting(val string)
	// GetMenuMediaSetting reads the custom media URL for ONE menu/category or
	// the alive card (Redis field "menumedia:<key>", e.g. menumedia:logo).
	// Returns def ("") when nothing is set for that key.
	GetMenuMediaSetting(key, def string) string
	// SetMenuMediaSetting stores the custom media URL for one menu key.
	SetMenuMediaSetting(key, url string)

	// ── PREFIX per-bot Redis config ──
	// GetPrefix reads the bot's command prefix from Redis (key prefix:<botJID>).
	// Returns def if not set / no Redis.
	GetPrefix(def string) string
	// SetPrefix writes the bot's command prefix to Redis. Empty string = no prefix.
	SetPrefix(prefix string)

	// NotifyPrefixChanged re-sends the GOLD-MD connected/startup card right
	// after a prefix change so the owner INSTANTLY sees the fresh prefix
	// (owner order: prefix change → connected msg foran fresh prefix ke sath).
	NotifyPrefixChanged()

	// MarkStatusRead marks a status (story) message as seen/read.
	// chat = status@broadcast JID, sender = the status owner, msgID = status
	// message ID. Equivalent to Baileys readMessages([key]).
	MarkStatusRead(chat, sender types.JID, msgID string) error
	// SendStatusReaction sends an emoji reaction to a status message.
	// statusOwner = the person who posted the status, msgID = status message
	// ID, emoji = the reaction emoji. Equivalent to Baileys sendMessage with
	// react + statusJidList.
	SendStatusReaction(statusOwner types.JID, msgID string, emoji string) error
	// SendStatusReply sends a private text reply to the person who posted a
	// status, quoting the status message so the receiver sees a status quote
	// preview. statusOwner = the person who posted the status (resolved PN JID),
	// msgID = the status message ID, text = the reply text,
	// quotedMsg = the unwrapped status message proto (for ContextInfo.QuotedMessage).
	// guardOn = true applies the lightweight-quote guard (single message, no
	// double delivery); false sends the raw quotedMsg (old behaviour).
	SendStatusReply(statusOwner types.JID, msgID string, text string, quotedMsg *waProto.Message, guardOn bool) error

	// -- GCSTATUS (post own status + mention to all groups) --
	// PostStatusToBroadcast posts the given message proto as the bot's own
	// WhatsApp status (story) by sending it to status@broadcast. The
	// mentionedGroupJIDs are embedded as additional meta nodes (mentioned_users)
	// so WhatsApp links the status to those groups (like the manual @ flow).
	// Returns the status message ID on success.
	PostStatusToBroadcast(msg *waProto.Message, mentionedGroupJIDs []types.JID) (string, error)
	// MentionStatusToGroup sends a GroupStatusMentionMessage to a single group,
	// referencing the just-posted status by its message ID. This produces the
	// "X mentioned your group in their status" notification inside the group
	// chat with the status preview -- exactly like the manual status -> @ ->
	// select groups flow.
	MentionStatusToGroup(groupJID types.JID, statusMsgID string) error
	// SendGroupStory sends the actual status media/content directly to a group
	// wrapped inside a GroupStatusMessageV2 (proto field 103, FutureProofMessage).
	// This creates the GREEN RING group story on the group profile picture.
	// mediaMsg = the actual image/video/audio/text proto (same as posted to status@broadcast).
	SendGroupStory(groupJID types.JID, mediaMsg *waProto.Message) error
	// GetJoinedGroupsList returns the JIDs of all groups the bot is a member of.
	GetJoinedGroupsList() ([]types.JID, error)

	// ── ANTI (antilink / antibot / antibad) per-group Redis config ──
	// GetGroupSetting reads a per-GROUP setting from Redis. groupJID is the
	// group JID (e.g. 120363...@g.us). Uses the settings:<groupJID> hash.
	GetGroupSetting(groupJID, field, def string) string
	// SetGroupSetting writes a per-GROUP setting to Redis.
	SetGroupSetting(groupJID, field, val string)
	// GroupSetAdd adds a member to a per-group Redis SET (e.g. antilink:<gid>:allowed).
	GroupSetAdd(groupJID, setName, member string) error
	// GroupSetRem removes a member from a per-group Redis SET.
	GroupSetRem(groupJID, setName, member string) error
	// GroupSetMembers returns all members of a per-group Redis SET.
	GroupSetMembers(groupJID, setName string) []string
	// GroupSetClear removes an entire per-group Redis SET (used on reset).
	GroupSetClear(groupJID, setName string) error

	// ── GROUP ADMIN / MODERATION actions ──
	// KickGroupMember removes the given user JIDs from the group.
	KickGroupMember(groupJID types.JID, targets []types.JID) error
	// DeleteAnyMessage revokes (deletes-for-everyone) a message in a chat by
	// its message ID and the original sender JID. Used by anti-* handlers to
	// delete the offending message. senderJID may be "" if it was the bot.
	DeleteAnyMessage(chat types.JID, senderJID string, messageID string) error
	// IsGroupAdmin reports whether the given user JID is an admin/super-admin
	// of the group identified by groupJID.
	IsGroupAdmin(groupJID, userJID types.JID) bool
	// ResolveToPN converts a LID (hidden user, @lid) JID to its real phone
	// number (PN) JID (@s.whatsapp.net). If the JID is already a PN, or the
	// mapping is unknown, the original JID is returned unchanged. This is the
	// MANDATORY LID->PN conversion used by every group command so that
	// mentions/replies that arrive in LID form are always acted on against the
	// real phone-number participant.
	ResolveToPN(jid types.JID) types.JID

	// ── MESSAGE inspection (for anti-* detection) ──
	// GetMessageText returns the full text body of the incoming message
	// identified by info, including image/video captions. "" if no text.
	GetMessageText(info types.MessageInfo) string
	// GetRawMessage returns the raw waProto.Message for the incoming message
	// identified by info (from the in-memory message cache). Returns nil if
	// the message is not cached. Used by antistatus enforcement to detect
	// group status mention messages (GroupStatusMentionMessage field).
	GetRawMessage(info types.MessageInfo) *waProto.Message
	// GetQuotedMessageText returns the text body of the quoted/replied-to
	// message (from ContextInfo.QuotedMessage), including captions. Returns
	// "" if the incoming message is not a reply or the quoted message has no
	// text. Used by .gcstatus to post a replied text message as a status.
	GetQuotedMessageText(info types.MessageInfo) string
	// GetQuotedMessageID extracts the quoted (replied-to) message ID and its
	// sender JID from the incoming message's ContextInfo. Returns
	// "","",false when the message is not a reply. Used by premium/botblock/
	// bangcuser to resolve a target from a replied-to message.
	GetQuotedMessageID(info types.MessageInfo) (string, string, bool)
	// IsForwardedBot reports whether the incoming message looks like a
	// forwarded bot/spam message (forwardingScore >= 2, or forwarded
	// newsletter, or isForwarded+stanzaId). Mirrors UmarIsBotForward.
	IsForwardedBot(info types.MessageInfo) bool

	// ── CUSTOM VOICE (addvoice) ──
	// SaveCustomVoice saves an audio clip under the given lowercase name.
	// data = raw audio bytes, mime = mimetype. Stored per-bot.
	SaveCustomVoice(name string, data []byte, mime string) bool
	// GetCustomVoice loads the audio bytes + mime for a saved voice name.
	// Returns nil,"",false if not found.
	GetCustomVoice(name string) ([]byte, string, bool)
	// DeleteCustomVoice removes a saved voice by name.
	DeleteCustomVoice(name string) bool
	// ListCustomVoices returns the names of all saved voices.
	ListCustomVoices() []string
	// SendVoiceMessage sends an audio clip (raw bytes) as a non-PTT audio
	// message to the chat identified by info.
	SendVoiceMessage(info types.MessageInfo, data []byte, mime string) error

	// ── CUSTOM ASSETS (.addimg/.addvideo/.addsticker/.addtext/.addcircle) ──
	// kind is one of: img, video, sticker, text, circle.
	// SaveCustomAsset stores bytes for a named asset (sidecar ".mime" + Redis
	// index), mirroring the .addvoice model. Returns false on error/empty.
	SaveCustomAsset(kind, name string, data []byte, mime string) bool
	// GetCustomAsset loads the bytes + mimetype of a named asset.
	GetCustomAsset(kind, name string) ([]byte, string, bool)
	// DeleteCustomAsset removes a named asset.
	DeleteCustomAsset(kind, name string) bool
	// SaveCustomAssetMeta is SaveCustomAsset + a metadata sidecar (.meta).
	// .addcircle uses it to remember seconds,width,height for replay.
	SaveCustomAssetMeta(kind, name string, data []byte, mime, meta string) bool
	// GetCustomAssetMeta loads bytes + mime + meta ("" when absent).
	GetCustomAssetMeta(kind, name string) ([]byte, string, string, bool)
	// ListCustomAssets returns the sorted names of every asset of a kind.
	ListCustomAssets(kind string) []string

	// SendCircleVideo uploads an ALREADY SQUARE mp4 (raw bytes) and sends it as
	// a WhatsApp circle video (Message.PtvMessage). thumbnail may be nil.
	SendCircleVideo(info types.MessageInfo, data []byte, seconds uint32, width uint32, height uint32, thumbnail []byte) error
	// SendCircleVideoFile is the streaming (path) variant of SendCircleVideo.
	SendCircleVideoFile(info types.MessageInfo, path string, seconds uint32, width uint32, height uint32, thumbnail []byte) error

	// GetAllConnectedClients returns every currently-connected WhatsApp
	// session (each = a unique WhatsApp number) the bot knows about. Used
	// by .chreact to react to a channel post from ALL sessions at once so
	// the reaction counter on the post increments per unique account
	// (one account = +1 on the counter). This is the "bot farm" mechanism.
	GetAllConnectedClients() []ConnectedClient

	// ── OWNER NAME / OWNER NUMBER / BOT NAME / ALIVE MSG (per-bot Redis) ──
	// GetOwnerNameSetting reads the owner display NAME from Redis
	// (field "ownername"). Returns def if not set.
	GetOwnerNameSetting(def string) string
	// SetOwnerNameSetting writes the owner display NAME to Redis.
	SetOwnerNameSetting(name string)
	// GetOwnerNumberSetting reads the owner WhatsApp NUMBER (digits) from
	// Redis (field "ownernumber"). Returns def if not set.
	GetOwnerNumberSetting(def string) string
	// SetOwnerNumberSetting writes the owner WhatsApp NUMBER to Redis.
	SetOwnerNumberSetting(number string)
	// GetSudoOwners returns the list of SUDO (additional) owner numbers
	// stored in Redis (field "sudowners", comma-separated digits).
	// The permanent (paired) owner is NOT included here.
	GetSudoOwners() []string
	// SetSudoOwners writes the full list of sudo owner numbers to Redis
	// (field "sudowners", comma-separated).
	SetSudoOwners(numbers []string)
	// IsSudoOwner reports whether the given number (digits only) is in the
	// sudo owners list OR is the permanent paired owner number.
	IsSudoOwner(number string) bool
	// GetPermanentOwnerNumber returns the bot's own paired number (digits),
	// which is the permanent owner that can never be deleted.
	GetPermanentOwnerNumber() string
	// GetBotNameSetting reads the bot display NAME from Redis
	// (field "botname"). Returns def if not set.
	GetBotNameSetting(def string) string
	// DelBotNameSetting removes the custom bot name from Redis so the
	// bot falls back to the branded default footer (used by .botname reset).
	DelBotNameSetting()
	// SetBotNameSetting writes the bot display NAME to Redis.
	SetBotNameSetting(name string)
	// GetAliveMsgSetting reads the custom .alive message text from Redis
	// (field "alivemsg"). Returns def ("") if not set.
	GetAliveMsgSetting(def string) string
	// SetAliveMsgSetting writes the custom .alive message text to Redis.
	// Empty string = use default alive message.
	SetAliveMsgSetting(msg string)

	// ── ANTIDELETE / ANTIEDIT (per-bot Redis: enabled + scope + mode) ──
	// GetAntiDeleteSetting reads the antidelete config from Redis.
	// Returns {enabled, scope, mode}. scope is "all" / "inbox" / "groups".
	// mode is "here" (send deleted msg to same chat) / "inbox" (send to bot's own DM).
	GetAntiDeleteSetting() AntiDeleteConfig
	// SetAntiDeleteSetting writes the antidelete config to Redis.
	SetAntiDeleteSetting(enabled bool, scope string)
	// GetAntiDeleteMode reads the antidelete delivery mode from Redis.
	// Returns "here" (default) or "inbox".
	GetAntiDeleteMode() string
	// SetAntiDeleteMode writes the antidelete delivery mode to Redis.
	// mode must be "here" or "inbox".
	SetAntiDeleteMode(mode string)
	// GetAntiEditSetting reads the antiedit config from Redis.
	GetAntiEditSetting() AntiDeleteConfig
	// SetAntiEditSetting writes the antiedit config to Redis.
	SetAntiEditSetting(enabled bool, scope string)
	// GetAntiEditMode reads the antiedit delivery mode from Redis.
	// Returns "here" (default) or "inbox".
	GetAntiEditMode() string
	// SetAntiEditMode writes the antiedit delivery mode to Redis.
	// mode must be "here" or "inbox".
	SetAntiEditMode(mode string)

	// ── ANTICALL (per-bot Redis: enabled + custom msg) ──
	// GetAntiCallSetting reads whether anticall is ON for this bot.
	// Returns false if not set (default: off, same as Node.js ANTI_CALL: false).
	GetAntiCallSetting() bool
	// SetAntiCallSetting writes the anticall on/off flag to Redis.
	SetAntiCallSetting(enabled bool)
	// GetAntiCallMessage reads the custom reject-call message from Redis.
	// Returns def if not set. def should be ANTICALL_DEFAULT_MSG.
	GetAntiCallMessage(def string) string
	// SetAntiCallMessage writes the custom reject-call message to Redis.
	// Empty string = reset to default (Redis-safe delete).
	SetAntiCallMessage(msg string)

	// ── Bot MEMORY (the same secure place the bot keeps antidelete /
	//    antiedit messages; the .automsg schedule is stored there too) ──
	// MemoryReady reports whether the bot's memory store is initialised and ready.
	MemoryReady() bool
	// MemorySave stores a JSON byte payload for the given automsg config id.
	// Used to persist a {duration, mode, msg, chat} schedule so it survives restarts.
	MemorySave(id string, data []byte) error
	// MemoryLoad retrieves the JSON config for an automsg id.
	// Returns (data, true, nil) if present; (nil, false, nil) if not set.
	MemoryLoad(id string) ([]byte, bool, error)
	// MemoryDelete removes the automsg config for an id (safe delete: idempotent,
	// returns nil if already gone). Used by .automsg once (after send) and
	// .automsg stop (cancel + delete config).
	MemoryDelete(id string) error
	// MemoryList enumerates every saved automsg schedule across all memory
	// shards and returns them as {ID, Data} pairs. Used by .automsg list /
	// .automsg delete so the owner can see all schedules and pick one by number.
	MemoryList() ([]AutomsgListItem, error)

	// ── Namespaced bot MEMORY (used by .dissmisstime / .admintime) ──
	// Same secure store as MemorySave, but under a caller-chosen namespace so
	// timed-admin timers live in their OWN space and never appear in .automsg
	// list. Key naming: <ns>/<id>.json
	MemorySaveNS(ns, id string, data []byte) error
	MemoryLoadNS(ns, id string) ([]byte, bool, error)
	MemoryDeleteNS(ns, id string) error
	MemoryListNS(ns string) ([]AutomsgListItem, error)

	// -- AUTOREACT / OWNERREACT (per-bot Redis config) --
	// SendReaction sends an emoji reaction to a message in a chat.
	// chat = the chat JID, sender = the original message sender JID,
	// msgID = the target message ID, emoji = the reaction emoji.
	// Equivalent to Baileys sendMessage({ react: { key, text: emoji } }).
	SendReaction(chat, sender types.JID, msgID, emoji string) error

	// -- WELCOME / GOODBYE (per-group Redis config) --
	// SendImageWithMentions sends an image (raw bytes) with a caption that
	// pings the supplied JIDs (ContextInfo.MentionedJID). Used by welcome/
	// goodbye to greet joiners/leavers with the group DP + a @mention.
	// Falls back to text-only if data is nil/empty.
	SendImageWithMentions(chat types.JID, data []byte, caption string, mentioned []string) error

	// GetGroupName returns the subject (name) of the group identified by
	// groupJID, fetching it from the WhatsApp server if needed. Returns
	// a fallback ("this group") on error.
	GetGroupName(groupJID types.JID) string

	// GetGroupProfilePicture downloads the group's profile picture (DP) as
	// raw JPEG bytes. Returns nil if the group has no picture or the fetch
	// fails — callers should fall back to a text-only welcome/goodbye.
	GetGroupProfilePicture(groupJID types.JID) []byte

	// SendTextWithMentions sends a plain text message that @-mentions the
	// given JIDs. Used as the text-only fallback for welcome/goodbye when
	// the group has no DP image.
	SendTextWithMentions(chat types.JID, text string, mentioned []string) error

	// ── BOT-WIDE BANNED USERS (botblock / botunblock / banlist) ──
	// Stored in Redis SET "banned:set" (JID membership) + a per-user
	// metadata JSON key for number/reason/bannedBy/bannedAt.
	BotBanAdd(jid, number, reason string) error
	BotBanRemove(jid string) error
	BotBanIsBanned(jid string) bool
	BotBanList() []BannedUserInfo
	BotBanMemberTails() []string // cached last-10-digit tails of all banned JIDs (0ms, number-tolerant check)

	// ── PREMIUM USERS (antilinkprem / antibotprem / antibadprem) ──
	// Stored in Redis SET "premium:set". Shared across all three prem cmds.
	PremiumAdd(jid, number string) error
	PremiumRemove(jid string) error
	PremiumList() []PremiumUserInfo
	PremiumIsMember(jid string) bool
	PremiumMemberTails() []string // cached last-10-digit tails of all premium JIDs (0ms, number-tolerant bypass)

	// ── PER-GROUP BANNED USERS (bangcuser / unbangcuser) ──
	// Stored in per-group Redis SET "bangcuser" + metadata hash per user.
	GroupBanUserAdd(groupJID, userJID, bannedBy string) error
	GroupBanUserRemove(groupJID, userJID string) error
	GroupBanUserIsBanned(groupJID, userJID string) bool
	GroupBanUserList(groupJID string) []GroupBannedUserInfo

	// ── AUTO READ (autoread) ──
	// Bot-wide setting stored in settings:<botJID> hash, field "autoread".
	// Values: "off" / "inbox" / "groups" / "all".
	GetAutoReadSetting(def string) string
	SetAutoReadSetting(mode string)

	// ── AUTOBLOCK (country-code auto-block) ──
	// GetAutoBlockSetting reads the autoblock on/off flag from Redis
	// settings:<botJID> hash, field "autoblock". Returns def if not set.
	GetAutoBlockSetting(def string) string
	// SetAutoBlockSetting writes the autoblock on/off flag ("on"/"off").
	SetAutoBlockSetting(val string)
	// GetAutoBlockCodes reads the comma-joined blocked country codes from
	// Redis settings:<botJID> hash, field "autoblock:codes". "" if none.
	GetAutoBlockCodes(def string) string
	// SetAutoBlockCodes writes the comma-joined country codes list.
	SetAutoBlockCodes(val string)
	// GetAutoBlockContactSave reads the contact-save mode ("on"/"off").
	// "on" = saved contacts (phone address book) are exempt from autoblock.
	GetAutoBlockContactSave(def string) string
	// SetAutoBlockContactSave writes the contact-save mode ("on"/"off").
	SetAutoBlockContactSave(val string)

	// ── AUTOREPLY (GOLD-MD AI auto-reply) ──
	// GetAutoReplyMode reads the autoreply mode from Redis settings:<botJID>
	// hash, field "autoreply". Values: "off" / "groups" / "inbox" / "on".
	GetAutoReplyMode(def string) string
	// SetAutoReplyMode writes the autoreply mode.
	SetAutoReplyMode(val string)
	// GetAutoReplyDelay reads the human-typing delay flag, field
	// "autoreply:delay". Values: "true" (enabled) / "false" (disabled).
	GetAutoReplyDelay(def string) string
	// SetAutoReplyDelay writes the human-typing delay flag.
	SetAutoReplyDelay(val string)
	// GetAutoReplyExcluded reads the comma-joined excluded-user JIDs from
	// Redis settings:<botJID> hash, field "autoreply:excluded". "" if none.
	GetAutoReplyExcluded(def string) string
	// SetAutoReplyExcluded writes the comma-joined excluded-user JID list.
	SetAutoReplyExcluded(val string)

	// ── BANCMD (stopped commands list) ──
	// GetBannedCommands reads the comma-joined stopped command names from
	// Redis settings:<botJID> hash, field "bancmd". "" if none.
	GetBannedCommands(def string) string
	// BanCommand appends name to the stopped-commands list (if absent).
	BanCommand(name string)
	// UnbanCommand removes name from the stopped-commands list (if present).
	UnbanCommand(name string)

	// ── QUOTED SEND (autoreply) ──
	// SendQuotedTextWithID sends text quoting the given message, WITHOUT
	// adding the bot footer. Returns the sent message ID ("" on failure).
	SendQuotedTextWithID(info types.MessageInfo, quoted *waProto.Message, text string) string

	// ── GROUP CHAT LOCK (bangc / unbangc) ──
	// Per-group setting stored in settings:<groupJID> hash, field "bangc".
	// Values: "on" (locked) / "off" (open). Uses GetGroupSetting/SetGroupSetting.

	// ── LOGO MENU ──
	// ShowLogoMenu renders the .logo command's fancy boxed menu (same format
	// as the other category menus) instead of plain text. Implemented by the
	// main package (manager.go CmdLogoMenu).
	ShowLogoMenu(info types.MessageInfo, args []string, prefix string)

	// ── FONT MENU ──
	// ShowFontMenu renders the .font command's fancy boxed menu (FONT1..
	// FONT1000) in the same format as the other category menus. Implemented
	// by the main package (manager.go CmdFontMenu).
	ShowFontMenu(info types.MessageInfo, args []string, prefix string)

	// ── EQUALIZER MENU ──
	// ShowEqualizerMenu renders the .equalizer command's fancy boxed menu
	// (EQ1..EQ1000) in the same format as the other category menus.
	// Implemented by the main package (manager.go CmdEqualizerMenu).
	ShowEqualizerMenu(info types.MessageInfo, args []string, prefix string)

	// ShowGameMenu renders the .game command's fancy boxed menu
	// (GAME1..GAME1000) in the same format as the other category menus.
	// Implemented by the main package (manager.go CmdGameMenu).
	ShowGameMenu(info types.MessageInfo, args []string, prefix string)
	// ShowBotStyleMenu renders the .botstyle command's fancy boxed menu
	// (BOTSTYLE1..BOTSTYLE50) in the same format as the other category
	// menus. Implemented by the main package (manager.go CmdBotStyleMenu).
	ShowBotStyleMenu(info types.MessageInfo, args []string, prefix string)
}

// BannedUserInfo holds the metadata for a bot-wide banned user.
type BannedUserInfo struct {
	JID      string
	Number   string
	Reason   string
	BannedBy string
	BannedAt string // ISO date string
}

// PremiumUserInfo holds the metadata for a premium (whitelist) user.
type PremiumUserInfo struct {
	JID    string
	Number string
}

// GroupBannedUserInfo holds the metadata for a per-group banned user.
type GroupBannedUserInfo struct {
	UserJID  string
	BannedBy string
}

// AutomsgListItem is one saved schedule returned by MemoryList.
// ID is the config key (botJID|chat); Data is the raw JSON config bytes.
type AutomsgListItem struct {
	ID   string
	Data []byte
}

// AntiDeleteConfig is the per-bot antidelete/antiedit configuration stored
// in Redis. Enabled + Scope (all / inbox / groups) + Mode (here / inbox).
// Same shape as the Node.js UmarGetAntiDeleteSetting {enabled, scope}, with
// an added Mode field for delivery destination of recovered deleted messages.
type AntiDeleteConfig struct {
	Enabled bool
	Scope   string // "all", "inbox", "groups"
	Mode    string // "here" (send to same chat) or "inbox" (send to bot's own DM)
}

// ConnectedClient is one connected WhatsApp session exposed to commands
// for multi-session operations (e.g. .chreact reacting from every number).
type ConnectedClient struct {
	JID    string            // the WhatsApp JID, e.g. 92...@s.whatsapp.net
	Client *whatsmeow.Client // the live whatsmeow client (connected)
}

// Command represents a registered bot command.
// Hidden = true means the command is an alias that still works but does NOT
// appear in the TOTAL COMMANDS count or the bot menu.
type Command struct {
	Name      string
	Category  string
	Desc      string
	OwnerOnly bool
	Hidden    bool
	Run       func(s SessionBridge, info types.MessageInfo, args []string, prefix string)
}

// CategoryOrder defines the display order of categories in the .menu.
// Categories not listed here are appended after, sorted alphabetically.
var CategoryOrder = []string{
	"OWNER & SYSTEM",
	"GROUP MANAGEMENT",
	"ANTI & PROTECTION",
	"DOWNLOADER",
	"AI",
	"AI & MEDIA",
	"PRESENCE & STATUS",
	"CONVERTER",
	"TOOLS",
	"BREACTION",
	"GREACTION",
	"EQUALIZER",
	"FONT",
	"GAME",
	"BOT STYLE",
}

// CategoryEmoji maps each category to a decorative emoji used in the menu header.
var CategoryEmoji = map[string]string{
	"OWNER & SYSTEM":    "🔰",
	"GROUP MANAGEMENT":  "🔰",
	"ANTI & PROTECTION": "🔰",
	"DOWNLOADER":        "🔰",
	"AI":                "🔰",
	"AI & MEDIA":        "🔰",
	"TOOLS":             "🔰",
	"PRESENCE & STATUS": "🔰",
	"CONVERTER":         "🔰",
	"BREACTION":         "🔰",
	"GREACTION":         "🔰",
	"EQUALIZER":         "🔰",
	"FONT":              "🔰",
	"GAME":              "🎮",
}

var registry []Command

// Register adds a command to the registry.
func Register(c Command) {
	registry = append(registry, c)
}

// Commands returns all registered commands (including hidden aliases).
func Commands() []Command {
	return registry
}

// CommandsCount returns the number of VISIBLE (non-hidden) commands.
// Hidden aliases (v, ytvideo, ytmp4, song, ytaudio, mp3) are excluded so the
// TOTAL COMMANDS count in the startup message only counts main commands.
func CommandsCount() int {
	count := 0
	for _, c := range registry {
		if !c.Hidden {
			count++
		}
	}
	return count
}

// OwnerOnlySet returns the set of command names (lower-cased) that are marked
// OwnerOnly. The main package's dispatch uses this to silently reject
// owner-only commands coming from non-owners.
func OwnerOnlySet() map[string]bool {
	out := make(map[string]bool, len(registry))
	for _, c := range registry {
		if c.OwnerOnly {
			out[strings.ToLower(c.Name)] = true
		}
	}
	return out
}
