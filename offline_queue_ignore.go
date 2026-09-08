package main

import (
	"sync"
	"time"
)

// ============================================================================
// GOLD-MD — Offline-Queue Message Ignore (permanent, silent)
// File: offline_queue_ignore.go
// ============================================================================
// Owner: "jab bot band tha jo bhi messages aaye the, online aane pe unka
// jawab NAHI dena — sirf online hone ke baad bheje gaye commands ka jawab."
//
// WhatsApp offline messages QUEUE karta hai — jab socket reconnect hota hai
// to saare purane queued messages ek saath aate hain aur bot un sab ka jawab
// de deta tha (stale replies ek saath dikhte the). Ye module un sab ko ROK
// deta hai: handler.go ke start me guard isOldMessage() check karta hai.
//
// Logic (per-JID, multi-session safe):
//   - Har successful connect (events.Connected) pe markOnlineFresh(jid) us
//     JID ka onlineFreshAt set karta hai + 300ms grace-window (buffered
//     pushes ke liye — network WhatsApp koi purana msg connect ke turant
//     baad push kar deta hai).
//   - HandleMessage me: agar message ka timestamp us JID ke onlineFreshAt
//     se pehle ka hai, ya grace-window ke andar aaya — ye PURANA (queued)
//     message hai → poora ignore, na command chale na reply jaye.
//   - Boot pe zero-value onlineFreshAt: sab messages naye maane jayenge
//     (correct — server start ke baad aane wale messages fresh hi hain).
//   - Silent: koi logging nahi (owner: logs bilkul nahi chahiye).
// ============================================================================

// offlineGrace: reconnect ke turant baad aane wale buffered messages ko
// bhi purana maan ne ka window (network WhatsApp connect pe offline-time
// ke kuch messages push kar deta hai).
const offlineGrace = 300 * time.Millisecond

// offlineState per-JID online-tracking state.
type offlineState struct {
	onlineFreshAt time.Time // last time this session came ONLINE (events.Connected)
	ignoreUntil   time.Time // messages ts < iske tak abhi ke window me ignore
}

var (
	offlineMu    sync.Mutex
	offlineStateMap = map[string]offlineState{}
)

// markOnlineFresh events.Connected pe call hota hai (manager.go) — is JID
// ki session ab ONLINE hai: ab se aane wale messages naye hain, iske
// pehle wale sab queued-purane ignore honge.
func markOnlineFresh(jid string) {
	now := time.Now()
	offlineMu.Lock()
	offlineStateMap[jid] = offlineState{
		onlineFreshAt: now,
		ignoreUntil:   now.Add(offlineGrace),
	}
	offlineMu.Unlock()
}

// isOldMessage batata hai: ye message is JID ki session ke offline-period
// ka hai ya reconnect ke grace-window ke andar aaya (buffered) — matlab
// PURANA, ignore karna hai. Boot ke baad ya online hone ke baad ka naya
// message false lauta dega (uska jawab normal chalega).
func isOldMessage(jid string, ts time.Time) bool {
	offlineMu.Lock()
	st, ok := offlineStateMap[jid]
	offlineMu.Unlock()
	// JID ka record nahi = pehli baar ya boot — kuch mat karo, naya message.
	if !ok {
		return false
	}
	// Case 1: message online aane se PEHLE ka hai — 100% queued purana.
	if ts.Before(st.onlineFreshAt) {
		return true
	}
	// Case 2: grace-window chal raha hai aur message us window ke andar
	// ka hai (buffered push) — purana maano.
	if time.Now().Before(st.ignoreUntil) && ts.Before(st.ignoreUntil) {
		return true
	}
	return false
}
