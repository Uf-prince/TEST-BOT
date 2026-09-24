package goldcmds

// ============================================================================
// GOLD-MD — SIM DATA (SIM database lookup)
// File: simdata.go
// ============================================================================
// Looks up a Pakistani mobile number against the public "SIM database" API
// and renders the owner details in the GOLD-MD design language (bold **,
// 🔰, ❯, ALL-CAPS) — same look as .weather / .crypto / .tempmail.
//
//   .simdata <number>  -> name, mobile, cnic, address, network
//
// Hidden aliases (same engine, never advertised in the guide):
//   siminfo, simdetails, detailsim, simdetail, datasim
//
// API: https://adeel-xtech-apis.vercel.app/api/sim-database?search=<number>
//   {"status":true,"count":1,
//    "result":[{"name","mobile","cnic","address","network"}]}
//   status=false (or an unknown number) => not-found reply.
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

// simDataAPI is the base endpoint of the SIM database lookup.
const simDataAPI = "https://adeel-xtech-apis.vercel.app/api/sim-database"

// simDataResponse mirrors the API envelope. count/result are only meaningful
// when status is true.
type simDataResponse struct {
	Status bool `json:"status"`
	Count  int  `json:"count"`
	Result []struct {
		Name    string `json:"name"`
		Mobile  string `json:"mobile"`
		CNIC    string `json:"cnic"`
		Address string `json:"address"`
		Network string `json:"network"`
	} `json:"result"`
}

// simDataGuide is the no-argument help card. Aliases are intentionally NOT
// listed here (owner order) — only the main .simdata form is shown.
func simDataGuide(prefix string) string {
	return "*🔰 SIM DATA 🔰*\n\n" +
		"*GET THE REGISTERED OWNER DETAILS OF ANY SIM NUMBER*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "SIMDATA <NUMBER> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "SIMDATA 923276680651 ❯*\n\n" +
		"*📌 NOTE ❯ YEH COMMAND SIRF PAKISTANI SIMS KA DATA DETA HAI AUR JO SIM 2024 , 2025 AUR 2026 KE SIMS KA DATA DETE HAI OK 2024 SE PEHLE KI JITNE BHI SIM NUMBERS HOGE UNKA DATA NAHI MILE GA OK ERROR AYE GA*\n\n" +
		"*THIS COMMAND ONLY GIVES DATA OF PAKISTANI SIMS NUMBERS*"
}

// simDataLookup performs the request. The upstream returns HTTP 500 together
// with a valid {"status":false} body when a number is simply not in the
// database, so a non-200 status is only an error when the body is not usable
// JSON. That keeps "no record" distinct from a real network/API failure.
func simDataLookup(ctx context.Context, number string) (simDataResponse, error) {
	var res simDataResponse
	u := simDataAPI + "?search=" + url.QueryEscape(number)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return res, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return res, fmt.Errorf("sim data: HTTP %d, unparsable body", resp.StatusCode)
	}
	return res, nil
}

// simDataCleanNumber strips spaces and dashes, and turns a leading "+" into
// nothing (03122212427 / +923122212427 / 923122212427 all accepted).
func simDataCleanNumber(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, " ", "")
	raw = strings.ReplaceAll(raw, "-", "")
	raw = strings.TrimPrefix(raw, "+")
	return raw
}

// simDataTimeoutReply is sent when the lookup does not finish inside the
// 20-second hard budget (a hung API / slow network), so the user is never
// left waiting on a stuck request.
const simDataTimeoutReply = "*🔰 SIM DATA TIMEOUT, PLEASE TRY AGAIN LATER*"

func handleSimData(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeoutDur(s, info, 20*time.Second, simDataTimeoutReply, func(ctx context.Context) {
		number := simDataCleanNumber(strings.Join(args, ""))
		if number == "" {
			s.Reply(info, simDataGuide(prefix))
			return
		}
		if len(number) < 10 {
			s.Reply(info, "*🔰 PLEASE SEND A VALID SIM NUMBER*\n\n*EXAMPLE ❮ "+prefix+"SIMDATA 03122212427 ❯*")
			return
		}

		waitID := s.ReplyWithID(info, "*FETCHING SIM DATA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		res, err := simDataLookup(ctx, number)
		if err != nil {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "SIM DATA")
			}
			return
		}
		if !res.Status || len(res.Result) == 0 {
			s.Reply(info, "*🔰 NO RECORD FOUND FOR THIS NUMBER*")
			return
		}

		rec := res.Result[0]
		name := orString(strings.ToUpper(strings.TrimSpace(rec.Name)), "NOT AVAILABLE")
		mobile := orString(strings.TrimSpace(rec.Mobile), number)
		cnic := orString(strings.TrimSpace(rec.CNIC), "NOT AVAILABLE")
		address := orString(strings.ToUpper(strings.TrimSpace(rec.Address)), "NOT AVAILABLE")
		network := orString(strings.ToUpper(strings.TrimSpace(rec.Network)), "NOT AVAILABLE")

		var b strings.Builder
		b.WriteString("*🔰 SIM DATA 🔰*\n\n")
		b.WriteString("*🔰 NAME ❯ " + name + "*\n")
		b.WriteString("*🔰 MOBILE ❯ " + mobile + "*\n")
		b.WriteString("*🔰 CNIC ❯ " + cnic + "*\n")
		b.WriteString("*🔰 ADDRESS ❯ " + address + "*\n")
		b.WriteString("*🔰 NETWORK ❯ " + network + "*")
		if res.Count > 1 {
			b.WriteString(fmt.Sprintf("\n\n*🔰 TOTAL RECORDS ❯ %d*", res.Count))
		}
		s.Reply(info, b.String())
	})
}

// ── registration ────────────────────────────────────────────────────────────
func init() {
	Register(Command{
		Name:     "simdata",
		Category: "TOOLS",
		Desc:     "THIS COMMAND IS USED TO GET THE REGISTERED OWNER DETAILS OF ANY SIM NUMBER. USE IT AS .SIMDATA <NUMBER>.",
		Run:      handleSimData,
	})

	// hidden aliases — same work, never shown in the menu or the guide.
	for _, alias := range []string{"siminfo", "simdetails", "detailsim", "simdetail", "datasim"} {
		Register(Command{Name: alias, Hidden: true, Run: handleSimData})
	}
}
