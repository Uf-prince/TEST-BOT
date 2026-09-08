package goldcmds

// ============================================================================
// GOLD-MD — .IMG / .PIN / .P command (SIMPLE VERSION)
// Ported from Node.js img.js — same text, same behaviour.
//
// Pinterest search → top 10 images seedha bhejta hai, koi
// number-selection nahi. Koi API key nahi chahiye.
//
// Pinterest ke internal BaseSearchResource API se (pin id + original
// image url seedha JSON me milte hain, koi HTML scrape/key nahi chahiye).
//
// NOTE: 👑 (crown) emoji in Node.js is replaced with 🔰 in GOLD-MD.
// ============================================================================

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	pinHelpTextShort = "*🔰 IMAGE DOWNLOAD  GUIDE 🔰*\n\n" +
		"*TYPE SAME LIKE THAT*\n" +
		"*IMG APPLES*\n*IMG CATS*\n\n*TO DOWNLOAD IMAGES*"

	pinRetryText = "*TRY AGAIN LATER*"
)

var pinUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// pinHTTPGet — HTTPS GET with browser headers, redirect follow.
func pinHTTPGet(target string, timeoutMs int, extraHeaders map[string]string) ([]byte, error) {
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", pinUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// pinSearchResult is one Pinterest pin with its image variants.
type pinSearchResult struct {
	Images struct {
		Orig struct {
			URL string `json:"url"`
		} `json:"orig"`
		X736 struct {
			URL string `json:"url"`
		} `json:"736x"`
		X474 struct {
			URL string `json:"url"`
		} `json:"474x"`
	} `json:"images"`
}

// pinSearch — query se Pinterest search (BaseSearchResource API).
func pinSearch(query string, limit int, timeoutMs int) ([]string, error) {
	q := strings.TrimSpace(query)
	sourceUrl := "/search/pins/?q=" + url.QueryEscape(q)
	// NOTE: JS does JSON.stringify({options:{query,scope:'pins'},context:{}})
	// — the query inside data is NOT escaped there either. We escape it the
	// same way JSON.stringify does (only quotes/backslashes) to keep 0% farak.
	qJSON := strings.ReplaceAll(q, `\`, `\\`)
	qJSON = strings.ReplaceAll(qJSON, `"`, `\"`)
	data := `{"options":{"query":"` + qJSON + `","scope":"pins"},"context":{}}`
	apiUrl := "https://www.pinterest.com/resource/BaseSearchResource/get/" +
		"?source_url=" + url.QueryEscape(sourceUrl) +
		"&data=" + url.QueryEscape(data) +
		"&_=" + strconv.FormatInt(time.Now().UnixMilli(), 10)

	headers := map[string]string{
		"Accept":                  "application/json, text/javascript, */*, q=0.01",
		"X-Requested-With":        "XMLHttpRequest",
		"X-Pinterest-PWS-Handler": "www/search/[scope].js",
		"Referer":                 "https://www.pinterest.com" + sourceUrl,
	}
	body, err := pinHTTPGet(apiUrl, timeoutMs, headers)
	if err != nil {
		return nil, err
	}

	var jsonResp struct {
		ResourceResponse struct {
			Data json.RawMessage `json:"data"`
		} `json:"resource_response"`
	}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("PIN_PAGE_SHORT")
	}

	// Pinterest kabhi 'data' ko seedha array bhejta hai, kabhi
	// { results: [...] } jaisa object — dono handle karo.
	var results []pinSearchResult
	trimmed := strings.TrimSpace(string(jsonResp.ResourceResponse.Data))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(jsonResp.ResourceResponse.Data, &results); err != nil {
			return nil, fmt.Errorf("PIN_NO_RESULTS")
		}
	} else {
		var resultsObj struct {
			Results json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(jsonResp.ResourceResponse.Data, &resultsObj); err != nil {
			return nil, fmt.Errorf("PIN_NO_RESULTS")
		}
		if len(resultsObj.Results) == 0 {
			return nil, fmt.Errorf("PIN_NO_RESULTS")
		}
		if err := json.Unmarshal(resultsObj.Results, &results); err != nil {
			return nil, fmt.Errorf("PIN_NO_RESULTS")
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("PIN_NO_RESULTS")
	}

	var urls []string
	for _, r := range results {
		u := ""
		if r.Images.Orig.URL != "" {
			u = r.Images.Orig.URL
		} else if r.Images.X736.URL != "" {
			u = r.Images.X736.URL
		} else if r.Images.X474.URL != "" {
			u = r.Images.X474.URL
		}
		if u == "" {
			continue
		}
		urls = append(urls, u)
		if len(urls) >= limit {
			break
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("PIN_NO_RESULTS")
	}
	return urls, nil
}

// pinDownloadImage — image download with Pinterest Referer
// (rejects <1000 bytes like img.js's PIN_TOO_SMALL).
func pinDownloadImage(target string, timeoutMs int) ([]byte, error) {
	client := &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", pinUserAgent)
	req.Header.Set("Referer", "https://www.pinterest.com/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) < 1000 {
		return nil, fmt.Errorf("PIN_TOO_SMALL")
	}
	return data, nil
}

func handleImg(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		s.Reply(info, pinHelpTextShort)
		return
	}

	waitMsgID := s.ReplyWithID(info, fmt.Sprintf(`*SEARCHING IMAGES FOR "%s"....*`, query))

	sentCount := 0
	searchErr := func() error {
		urls, err := pinSearch(query, 10, 45000)
		if err != nil {
			return err
		}
		for _, u := range urls {
			// JS: .png → image/png, else image/jpeg (SendImage hardcodes
			// image/jpeg in this bridge — same visible behaviour for pins).
			data, err := pinDownloadImage(u, 60000)
			if err != nil {
				continue // JS: single-image failure just skips to the next
			}
			if err := s.SendImage(info, data, ""); err != nil {
				continue // JS: UmarSendOneImage catch → return false (skip)
			}
			sentCount++
		}
		return nil
	}()

	// JS finally: wait message delete hamesha hota hai (success OR error).
	if waitMsgID != "" {
		s.DeleteMessage(info, waitMsgID)
	}

	if searchErr != nil || sentCount == 0 {
		s.Reply(info, pinRetryText)
	}
}

func init() {
	Register(Command{Name: "img", Category: "DOWNLOADER", Desc: "Search Images for a topic/query and send the top matching images directly in this chat.", Run: handleImg})
	Register(Command{Name: "pin", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
	Register(Command{Name: "image", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
	Register(Command{Name: "images", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
	Register(Command{Name: "pics", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
	Register(Command{Name: "photo", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
	Register(Command{Name: "imgdl", Category: "DOWNLOADER", Hidden: true, Run: handleImg})
}
