package goldcmds

// ============================================================================
// GOLD-MD — Clear Chat Command  (interactive 1 / 2 flow)
// File: clear.go
// ============================================================================
// COMMAND: .clear    (owner-only)
//
// FLOW (owner order):
//   1. Owner types `.clear`.
//   2. The `.clear` command message is DELETED FOR EVERYONE.
//   3. The bot sends the prompt:
//        *TYPE ❮ 1 ❯ TO CLEAR ALL MESSAGES INCLUDE STARRED MESSAGES*
//        *TYPE ❮ 2 ❯ TO CLEAR ALL MESSAGES WITHOUT STARRED*
//   4. Owner replies with `1` or `2`.
//   5. The bot clears the chat according to the choice, then DELETES the
//      prompt message AND the owner's `1`/`2` reply message (both for
//      everyone), so nothing is left behind.
//
//   ❮ 1 ❯ → clear EVERYTHING (starred messages included)
//   ❮ 2 ❯ → clear everything EXCEPT starred messages (starred kept)
//
// whatsmeow: this uses the WhatsApp "clear chat" app-state mutation
//   (regular_high, apiVersion 6) with index
//   ["clearChat", <jid>, <flag>, "0"] where:
//       flag "1" → clear EVERYTHING (starred messages included)
//       flag "0" → clear everything EXCEPT starred messages (starred kept)
//   This mirrors Baileys' chatModify({ clear: ... }) implementation exactly.
//
//   NOTE: the previous implementation used appstate.BuildDeleteChat(...) whose
//   `deleteMedia` flag only controls MEDIA deletion — it does NOT control
//   starred-message retention, which is why BOTH `1` and `2` wiped starred
//   messages. The clearChat index's 3rd element is the real starred switch.
//
// Aliases (Hidden): clearchat, purge
// ============================================================================

import (
	"context"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
)

// clearSessionTTL: how long the bot waits for the owner's 1/2 reply.
const clearSessionTTL = 60 * time.Second

// clearPromptText is the exact prompt the owner asked for.
const clearPromptText = "*TYPE ❮ 1 ❯ TO CLEAR ALL MESSAGES INCLUDE STARRED MESSAGES*\n" +
	"*TYPE ❮ 2 ❯ TO CLEAR ALL MESSAGES WITHOUT STARRED*"

type clearSession struct {
	PromptID  string
	CreatedAt time.Time
}

var (
	clearSessions   = make(map[string]*clearSession)
	clearSessionsMu sync.Mutex
)

func clearKey(s SessionBridge, info types.MessageInfo) string {
	return s.GetJID() + "|" + info.Chat.String()
}

func setClearSession(key string, sess *clearSession) {
	clearSessionsMu.Lock()
	defer clearSessionsMu.Unlock()
	sess.CreatedAt = time.Now()
	clearSessions[key] = sess
}

func getClearSession(key string) *clearSession {
	clearSessionsMu.Lock()
	defer clearSessionsMu.Unlock()
	sess, ok := clearSessions[key]
	if !ok {
		return nil
	}
	if time.Since(sess.CreatedAt) > clearSessionTTL {
		delete(clearSessions, key)
		return nil
	}
	return sess
}

func handleClear(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	go handleClearAsync(s, info, args, prefix)
}

func handleClearAsync(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	if !s.IsOwner(info) {
		return
	}

	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		s.Reply(info, "🔰 *Client not connected.*")
		return
	}

	// 1) Delete the `.clear` command message itself (for everyone).
	_ = s.DeleteMessage(info, info.ID)

	// 2) Send the 1 / 2 prompt and remember its message ID.
	promptID := s.ReplyWithID(info, clearPromptText)

	// 3) Open a pending clear session for this chat.
	setClearSession(clearKey(s, info), &clearSession{PromptID: promptID})
}

// ClearTryHandle intercepts the owner's `1` / `2` reply for a pending `.clear`
// prompt. Returns true when the message was consumed (so the dispatcher stops).
// Mirrors the CompressTryHandle / SettingsTryHandle session pattern.
func ClearTryHandle(s SessionBridge, info types.MessageInfo, body string, prefix string) bool {
	key := clearKey(s, info)
	sess := getClearSession(key)
	if sess == nil {
		return false
	}

	trimmed := strings.TrimSpace(body)
	if trimmed != "1" && trimmed != "2" {
		return false
	}

	// Claim the session (delete) so a double reply can't trigger twice.
	clearSessionsMu.Lock()
	if cur, ok := clearSessions[key]; !ok || cur != sess {
		clearSessionsMu.Unlock()
		return false
	}
	delete(clearSessions, key)
	clearSessionsMu.Unlock()

	includeStarred := trimmed == "1"
	go clearRun(s, info, sess.PromptID, includeStarred)
	return true
}

// clearRun performs the actual clear and cleans up the prompt + reply messages.
func clearRun(s SessionBridge, info types.MessageInfo, promptID string, includeStarred bool) {
	client := s.GetClient()
	if client == nil || !client.IsConnected() {
		return
	}

	// Delete the prompt message AND the owner's `1`/`2` reply (for everyone).
	if promptID != "" {
		_ = s.DeleteMessage(info, promptID)
	}
	_ = s.DeleteMessage(info, info.ID)

	chat := info.Chat
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// WhatsApp "clear chat" app-state mutation (regular_high, apiVersion 6).
	// Index = ["clearChat", <jid>, <flag>, "0"] where:
	//   flag "1" → clear EVERYTHING (starred messages included)
	//   flag "0" → clear everything EXCEPT starred messages (starred kept)
	clearFlag := "1"
	if !includeStarred {
		clearFlag = "0"
	}

	patch := appstate.PatchInfo{
		Type: appstate.WAPatchRegularHigh,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexClearChat, chat.String(), clearFlag, "0"},
			Version: 6,
			Value: &waSyncAction.SyncActionValue{
				ClearChatAction: &waSyncAction.ClearChatAction{
					MessageRange: &waSyncAction.SyncActionMessageRange{
						LastMessageTimestamp: proto.Int64(time.Now().Unix()),
					},
				},
			},
		}},
	}
	_ = client.SendAppState(ctx, patch)
}

func init() {
	Register(Command{Name: "clear", Category: "OWNER & SYSTEM", Desc: "THIS COMMAND IS USED TO CLEAR ALL MESSAGES OF THE CHAT. USE IT IN THE CHAT YOU WANT TO CLEAR.", OwnerOnly: true, Run: handleClear})
	Register(Command{Name: "clearchat", OwnerOnly: true, Hidden: true, Run: handleClear})
	Register(Command{Name: "purge", OwnerOnly: true, Hidden: true, Run: handleClear})
}
