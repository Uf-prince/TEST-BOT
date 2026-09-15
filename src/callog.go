package main

// callog.go — LIVE call-signaling JSON capture (owner request).
//
// Purpose: jab owner bot ko call karke END karega, har call-related node
// (in aur out dono) ek JSON line ban kar /workspace/calldebug.jsonl me
// likha jayega. Isse dono flows compare honge:
//   - IN-*  — WhatsApp ne humein kya bheja (offer / terminate / reject / ...)
//   - OUT-* — humne WhatsApp ko kya bheja (hamare terminate / reject stanzas)
//
// Compare karke exact asli stanza shape milegi jo baad me pakka
// full-close command banane me use hogi.
//
// Activated ONLY by GOLDMD_CALL_DEBUG=1 (restart ke sath). Default off —
// production me zero overhead (ek bool check, file kabhi nahi khulti).

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// callDebugEnabled is set once at boot from GOLDMD_CALL_DEBUG.
var callDebugEnabled = os.Getenv("GOLDMD_CALL_DEBUG") == "1"

// callDebugMu guards the file open (whatsmeow dispatches each event in its
// own goroutine, so captures can race).
var callDebugMu sync.Mutex

// callDebugFile opens /workspace/calldebug.jsonl in append mode.
func callDebugFile() *os.File {
	callDebugMu.Lock()
	defer callDebugMu.Unlock()
	f, err := os.OpenFile("/workspace/calldebug.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	return f
}

// nodeToMaps recursively converts a waBinary.Node into a clean JSON-able
// map: attrs as strings (JID objects rendered via .String()), children as
// nested maps, byte content as base64 + printable preview.
func nodeToMaps(n *waBinary.Node) map[string]any {
	if n == nil {
		return nil
	}
	out := map[string]any{"tag": n.Tag}
	if len(n.Attrs) > 0 {
		attrs := map[string]string{}
		for k, v := range n.Attrs {
			attrs[k] = attrStr(v)
		}
		out["attrs"] = attrs
	}
	if data, ok := n.Content.([]byte); ok {
		prev := data
		if len(prev) > 48 {
			prev = prev[:48]
		}
		out["content_b64"] = base64.StdEncoding.EncodeToString(data)
		out["content_preview"] = printable(prev)
	}
	kids := n.GetChildren()
	if len(kids) > 0 {
		childList := make([]any, 0, len(kids))
		for i := range kids {
			childList = append(childList, nodeToMaps(&kids[i]))
		}
		out["children"] = childList
	}
	return out
}

// attrStr renders an attr value as a string for JSON.
func attrStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case types.JID:
		return x.String()
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// printable renders bytes with non-ASCII replaced by dots.
func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 32 && c < 127 {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

// fillMeta adds the common BasicCallMeta fields to a capture record.
func fillMeta(fields map[string]any, m types.BasicCallMeta) {
	if !m.From.IsEmpty() {
		fields["from"] = m.From.String()
	}
	fields["call_id"] = m.CallID
	if !m.CallCreator.IsEmpty() {
		fields["creator"] = m.CallCreator.String()
	}
	if !m.CallCreatorAlt.IsEmpty() {
		fields["creator_alt"] = m.CallCreatorAlt.String()
	}
	if !m.GroupJID.IsEmpty() {
		fields["group"] = m.GroupJID.String()
	}
	if !m.Timestamp.IsZero() {
		fields["ts_wa"] = m.Timestamp.UTC().Format(time.RFC3339)
	}
}

// CallDebugIn — capture an INBOUND call event (WhatsApp → hum) with its
// raw Data node. dir=IN.
func CallDebugIn(stage string, evt any) {
	if !callDebugEnabled {
		return
	}
	f := callDebugFile()
	if f == nil {
		return
	}
	defer f.Close()
	fields := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"dir":   "IN",
		"stage": stage,
	}
	switch e := evt.(type) {
	case *events.CallOffer:
		fillMeta(fields, e.BasicCallMeta)
		fields["remote_platform"] = e.RemotePlatform
		fields["remote_version"] = e.RemoteVersion
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallOfferNotice:
		fillMeta(fields, e.BasicCallMeta)
		fields["media"] = e.Media
		fields["type"] = e.Type
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallTerminate:
		fillMeta(fields, e.BasicCallMeta)
		fields["reason"] = e.Reason
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallReject:
		fillMeta(fields, e.BasicCallMeta)
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallAccept:
		fillMeta(fields, e.BasicCallMeta)
		fields["remote_platform"] = e.RemotePlatform
		fields["remote_version"] = e.RemoteVersion
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallPreAccept:
		fillMeta(fields, e.BasicCallMeta)
		fields["remote_platform"] = e.RemotePlatform
		fields["remote_version"] = e.RemoteVersion
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallTransport:
		fillMeta(fields, e.BasicCallMeta)
		fields["remote_platform"] = e.RemotePlatform
		fields["remote_version"] = e.RemoteVersion
		fields["node"] = nodeToMaps(e.Data)
	case *events.CallRelayLatency:
		fillMeta(fields, e.BasicCallMeta)
		fields["node"] = nodeToMaps(e.Data)
	case *events.UnknownCallEvent:
		fields["node"] = nodeToMaps(e.Node)
	default:
		fields["event"] = "untyped"
	}
	writeCallDebugLine(f, fields)
}

// CallDebugOut — capture an OUTBOUND node (hum → WhatsApp) with the send
// result. dir=OUT.
func CallDebugOut(label string, n *waBinary.Node, err error) {
	if !callDebugEnabled {
		return
	}
	f := callDebugFile()
	if f == nil {
		return
	}
	defer f.Close()
	fields := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"dir":   "OUT",
		"stage": label,
		"node":  nodeToMaps(n),
	}
	if err != nil {
		fields["error"] = err.Error()
	} else {
		fields["result"] = "sent_ok"
	}
	writeCallDebugLine(f, fields)
}

// writeCallDebugLine marshals fields and appends ONE JSON line.
func writeCallDebugLine(f *os.File, fields map[string]any) {
	data, err := json.Marshal(fields)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = f.Write(data)
}
