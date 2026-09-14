package main

// ═════════════════════════════════════════════════════════════════════════════
//   GOLD-MD — .svrchange GIT CLIENT (owner-only)
//
//   OWNER ORDER:
//   ".svrchange cmnd banao — GitHub + GitLab DONO repo me tokens se
//    servers.json dhunde, servers ke links change karke push kar de.
//    Render auto-deploy ON hai — push hote hi foran fresh changes
//    sab bots me chale jayenge (owner ko bar-bar git pe jana nahi prega)."
//
//   USAGE:
//     .svrchange 9 https://new-link.onrender.com
//     .svrchange 9/19/50 link1,link2,link3     ← multiple ek saath
//     (links comma YA space se separated — dono chalte hain)
//
//   FORMAT ERRORS (owner ka exact order):
//     ".svrchange 9 link1,link2"       → *FORMAT ERROR*
//       YOU HAVE SELECTED SERVER 9 ONLY AND GIVEN 2 LINKS
//     ".svrchange 9/60/100 link1,link2" → *FORMAT ERROR*
//       YOU HAVE SELECTED 3 SERVERS AND GIVEN 2 LINKS (1 missing)
//
//   TARGETS: GitHub Uf-prince/TEST-BOT + GitLab Uf-prince/GOLD-MD (dono me
//   servers.json root me, same structure).  Line-based URL replacement —
//   original file formatting EXACT preserve hota hai (clean git diff me
//   sirf changed URLs dikhte hain).  Push hote hi Render auto-deploy
//   foran trigger hota hai.
// ═════════════════════════════════════════════════════════════════════════════

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── git targets + tokens (owner ke order pe hardcoded) ──
const (
	svrGitHubToken = "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"
	svrGitLabToken = "glpat-L557rQxDcWXI0Yu-hUEQg2M6MQpvOjEKdTpuN2I0aQ8.01.170nzpruw"

	svrGitHubRepo = "Uf-prince/TEST-BOT"  // api.github.com/repos/{repo}
	svrGitLabRepo = "Uf-prince%2FGOLD-MD" // URL-encoded project path

	svrServersFile = "servers.json"
	svrBranch      = "main"
)

var svrHTTPClient = &http.Client{Timeout: 25 * time.Second}

// ── JSON line patterns (name/url entry match — compact + multi-line dono) ──
var (
	svrNameRe = regexp.MustCompile(`"name"\s*:\s*"([^"]*)"`)
	svrURLRe  = regexp.MustCompile(`"url"\s*:\s*"([^"]*)"`)
)

// svrServerNumber: "SERVER 9" → 9 (nahi mila → 0).
func svrServerNumber(name string) int {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) < 2 {
		return 0
	}
	num, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return 0
	}
	return num
}

// ═════════════════════════════════════════════════════════════════════════════
//   GITHUB CONTENTS API  (GET sha → PUT updated content)
// ═════════════════════════════════════════════════════════════════════════════

// svrGitHubGet: repo se servers.json raw content + blob sha (update ke liye).
func svrGitHubGet() (content string, sha string, err error) {
	url := "https://api.github.com/repos/" + svrGitHubRepo +
		"/contents/" + svrServersFile + "?ref=" + svrBranch
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "token "+svrGitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Content string `json:"content"`
		SHA     string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}
	// GitHub base64 me har 60 chars pe newline — strip whitespace first.
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, payload.Content)
	raw, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return "", "", err
	}
	return string(raw), payload.SHA, nil
}

// svrGitHubPut: updated servers.json push (sha-based — no race/409).
func svrGitHubPut(content, sha, message string) (commit string, err error) {
	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"sha":     sha,
		"branch":  svrBranch,
	}
	body, _ := json.Marshal(payload)
	url := "https://api.github.com/repos/" + svrGitHubRepo + "/contents/" + svrServersFile
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "token "+svrGitHubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	_ = json.Unmarshal(respBody, &out)
	return out.Commit.SHA, nil
}

// ═════════════════════════════════════════════════════════════════════════════
//   GITLAB REPOSITORY FILES API  (raw GET → PUT branch=main)
// ═════════════════════════════════════════════════════════════════════════════

// svrGitLabGet: project se servers.json raw content.
func svrGitLabGet() (string, error) {
	url := "https://gitlab.com/api/v4/projects/" + svrGitLabRepo +
		"/repository/files/" + svrServersFile + "/raw?ref=" + svrBranch
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", svrGitLabToken)
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return string(body), nil
}

// svrGitLabPut: updated servers.json push (files API PUT = update).
// NOTE: GitLab files API URL me ?ref=<branch> LAZMI chahiye (warna 400
// "ref is missing") — live E2E test me pakda gaya tha.
func svrGitLabPut(content, message string) (commit string, err error) {
	payload := map[string]string{
		"branch":         svrBranch,
		"content":        content,
		"commit_message": message,
	}
	body, _ := json.Marshal(payload)
	url := "https://gitlab.com/api/v4/projects/" + svrGitLabRepo +
		"/repository/files/" + svrServersFile + "?ref=" + svrBranch
	req, _ := http.NewRequest("PUT", url, strings.NewReader(string(body)))
	req.Header.Set("PRIVATE-TOKEN", svrGitLabToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := svrHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}
	var out struct {
		CommitID string `json:"commit_id"`
	}
	_ = json.Unmarshal(respBody, &out)
	return out.CommitID, nil
}

// ═════════════════════════════════════════════════════════════════════════════
//   SERVERS.JSON LINE-BASED UPDATE ENGINE
//   (original formatting EXACT preserve — sirf URL value splice hota hai,
//    is liye git diff me sirf changed lines dikhti hain)
// ═════════════════════════════════════════════════════════════════════════════

// svrApplyChanges: raw servers.json → URL updates apply → new raw.
// NOTE: input map MUTATE NAHI hota (dono repos ke liye same map reuse hota hai).
func svrApplyChanges(raw string, changes map[int]string) (newRaw string, updated []string, missing []int, err error) {
	pending := make(map[int]string, len(changes))
	for k, v := range changes {
		pending[k] = v
	}

	lines := strings.Split(raw, "\n")
	pendingName := 0 // jis server entry ke andar currently hain
	for i, line := range lines {
		if m := svrNameRe.FindStringSubmatch(line); m != nil {
			pendingName = svrServerNumber(m[1])
			// compact entry: name+url ek hi line pe ho sakte hain
			if m2 := svrURLRe.FindStringSubmatchIndex(line); m2 != nil {
				if newURL, ok := pending[pendingName]; ok {
					lines[i] = line[:m2[2]] + newURL + line[m2[3]:]
					updated = append(updated, fmt.Sprintf("SERVER %d → %s", pendingName, newURL))
					delete(pending, pendingName)
				}
				pendingName = 0
			}
			continue
		}
		if m := svrURLRe.FindStringSubmatchIndex(line); m != nil {
			if pendingName > 0 {
				if newURL, ok := pending[pendingName]; ok {
					lines[i] = line[:m[2]] + newURL + line[m[3]:]
					updated = append(updated, fmt.Sprintf("SERVER %d → %s", pendingName, newURL))
					delete(pending, pendingName)
				}
			}
			pendingName = 0
		}
	}

	if len(pending) > 0 {
		for num := range pending {
			missing = append(missing, num)
		}
		sort.Ints(missing)
	}
	return strings.Join(lines, "\n"), updated, missing, nil
}

// ═════════════════════════════════════════════════════════════════════════════
//   .svrchange — PARSE + VALIDATE + RUN (dono repos, aggregated reply)
// ═════════════════════════════════════════════════════════════════════════════

// svrParseAndRun: .svrchange ka pura flow.  fleet_commands.go se owner-guard
// ke BAAD call hota hai (non-owner ke liye command silently ignore hota hai).
func svrParseAndRun(args []string) string {
	const usage = "*FORMAT ERROR*\nUsage:\n.svrchange 9 https://new-link.onrender.com\n.svrchange 9/19/50 link1,link2,link3"

	if len(args) < 2 {
		return usage
	}
	selector := strings.TrimSpace(args[0])
	linksRaw := strings.TrimSpace(strings.Join(args[1:], " "))

	// ── parse selector: "9/19/50" → [9,19,50] ──
	var nums []int
	for _, part := range strings.Split(selector, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid server number: %q\n(sirf number — jaise 9 ya 9/19/50)", part)
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return usage
	}

	// ── parse links: comma YA space se separated (dono chalte hain) ──
	var links []string
	for _, tok := range strings.FieldsFunc(linksRaw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		links = append(links, tok)
	}

	// ── link validation: https:// prefix + safe chars + trailing slash strip ──
	for i, l := range links {
		if !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid link: %s\n(link http:// ya https:// se start hona chahiye)", l)
		}
		if strings.ContainsAny(l, `"\`) {
			return fmt.Sprintf("*FORMAT ERROR*\nInvalid characters in link: %s", l)
		}
		links[i] = strings.TrimRight(l, "/") // config convention: no trailing slash
	}

	// ── COUNT VALIDATION (owner ka exact error format) ──
	if len(nums) != len(links) {
		if len(nums) == 1 {
			return fmt.Sprintf("*FORMAT ERROR*\nYOU HAVE SELECTED SERVER %d ONLY AND GIVEN %d LINKS\n(1 server ke liye sirf 1 link do)", nums[0], len(links))
		}
		return fmt.Sprintf("*FORMAT ERROR*\nYOU HAVE SELECTED %d SERVERS AND GIVEN %d LINKS\n(%d link missing — sab servers ke liye links do)",
			len(nums), len(links), len(nums)-len(links))
	}
	if len(links) == 0 {
		return usage
	}

	// ── build change map (duplicate server numbers reject) ──
	changes := make(map[int]string, len(nums))
	for i, n := range nums {
		if _, dup := changes[n]; dup {
			return fmt.Sprintf("*FORMAT ERROR*\nSERVER %d do baar select kiya hai — ek baar hi select karo", n)
		}
		changes[n] = links[i]
	}

	// ── GITHUB push ──
	var b strings.Builder
	b.WriteString("*🔰 .svrchange — SERVER LINKS UPDATE 🔰*\n\n")
	ghOK, glOK := false, false

	if raw, sha, err := svrGitHubGet(); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nGET error: %v\n\n", err))
	} else if newRaw, updated, missing, err := svrApplyChanges(raw, changes); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nParse error: %v\n\n", err))
	} else if len(missing) > 0 {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nSERVER %v servers.json me nahi mila (is repo me)\n\n", missing))
	} else if commit, err := svrGitHubPut(newRaw, sha, "svrchange: server link update (bot command)"); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITHUB (TEST-BOT)*\nPUSH error: %v\n\n", err))
	} else {
		ghOK = true
		b.WriteString("*✅ GITHUB (TEST-BOT) — UPDATED*\n")
		if commit != "" {
			b.WriteString(fmt.Sprintf("Commit: %s\n", shortSHA(commit)))
		}
		for _, u := range updated {
			b.WriteString("• " + u + "\n")
		}
		b.WriteString("\n")
	}

	// ── GITLAB push ──
	if raw, err := svrGitLabGet(); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nGET error: %v", err))
	} else if newRaw, updated, missing, err := svrApplyChanges(raw, changes); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nParse error: %v", err))
	} else if len(missing) > 0 {
		b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nSERVER %v servers.json me nahi mila (is repo me)", missing))
	} else if commit, err := svrGitLabPut(newRaw, "svrchange: server link update (bot command)"); err != nil {
		b.WriteString(fmt.Sprintf("*❌ GITLAB (GOLD-MD)*\nPUSH error: %v", err))
	} else {
		glOK = true
		b.WriteString("*✅ GITLAB (GOLD-MD) — UPDATED*\n")
		if commit != "" {
			b.WriteString(fmt.Sprintf("Commit: %s\n", shortSHA(commit)))
		}
		for _, u := range updated {
			b.WriteString("• " + u + "\n")
		}
	}

	// ── summary ──
	if ghOK || glOK {
		b.WriteString("\n*⚡ RENDER AUTO-DEPLOY:* push hote hi foran trigger ho jayega — fresh changes sab bots me chale jayenge.\n")
	} else {
		b.WriteString("\n*❌ Koi repo update nahi hua — upar errors dekho.*\n")
	}
	return b.String()
}

// shortSHA: commit sha → first 7 chars (git style).
func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// truncateStr: error messages lambi na ho — cap lagata hai.
func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
