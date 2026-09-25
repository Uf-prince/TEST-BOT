package goldcmds

// ============================================================================
// GOLD-MD — TOOLS PACK 11 (10 new everyday commands)
// File: toolpack11.go
// ============================================================================
//   .tvmaze <query>      -> TV show information
//   .openlibrary <query> -> book information
//   .jikan <query>       -> anime information
//   .githubuser <user>   -> GitHub user profile
//   .officialjoke        -> random joke with punchline
//   .pypiinfo <package>  -> PyPI package information
//   .ayah <number>       -> Quran ayah by number
//   .wikisearch <query>  -> Wikipedia summary
//   .linkpreview <url>   -> preview of any website link
//   .kanyerest           -> random Kanye West quote
//
// All use FREE public APIs (no key) and match the GOLD-MD design language
// exactly (bold **, 🔰, ❮ ❯, ALL-CAPS).
// ============================================================================

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// ── .TVMAZE ─────────────────────────────────────────────────────────────────

func tvmazeGuide(prefix string) string {
	return "*🔰 TV SHOW INFO 🔰*\n\n" +
		"*GET INFORMATION ABOUT ANY TV SHOW*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "TVMAZE <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "TVMAZE GIRLS ❯*"
}

func handleTvmaze(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, tvmazeGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING SHOW....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res []struct {
			Show struct {
				Name      string   `json:"name"`
				Type      string   `json:"type"`
				Language  string   `json:"language"`
				Genres    []string `json:"genres"`
				Status    string   `json:"status"`
				Premiered string   `json:"premiered"`
				Rating    struct {
					Average float64 `json:"average"`
				} `json:"rating"`
				Network struct {
					Name string `json:"name"`
				} `json:"network"`
				Image struct {
					Medium string `json:"medium"`
				} `json:"image"`
			} `json:"show"`
		}
		u := "https://api.tvmaze.com/search/shows?q=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO SHOW FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		sh := res[0].Show
		var b strings.Builder
		b.WriteString("*🔰 TV SHOW INFO 🔰*\n\n")
		b.WriteString("*📺 NAME ❯ " + strings.ToUpper(sh.Name) + "*\n")
		if sh.Type != "" {
			b.WriteString("*🎭 TYPE ❯ " + strings.ToUpper(sh.Type) + "*\n")
		}
		if len(sh.Genres) > 0 {
			b.WriteString("*🏷️ GENRES ❯ " + strings.ToUpper(strings.Join(sh.Genres, ", ")) + "*\n")
		}
		if sh.Status != "" {
			b.WriteString("*📌 STATUS ❯ " + strings.ToUpper(sh.Status) + "*\n")
		}
		if sh.Premiered != "" {
			b.WriteString("*📅 PREMIERED ❯ " + sh.Premiered + "*\n")
		}
		if sh.Network.Name != "" {
			b.WriteString("*🏢 NETWORK ❯ " + strings.ToUpper(sh.Network.Name) + "*\n")
		}
		if sh.Rating.Average > 0 {
			b.WriteString("*⭐ RATING ❯ " + fmt.Sprintf("%.1f/10", sh.Rating.Average) + "*")
		}
		if sh.Image.Medium != "" {
			if data, err := funGetBytes(ctx, sh.Image.Medium); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .OPENLIBRARY ────────────────────────────────────────────────────────────

func openlibraryGuide(prefix string) string {
	return "*🔰 BOOK INFO 🔰*\n\n" +
		"*GET INFORMATION ABOUT ANY BOOK*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "OPENLIBRARY <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "OPENLIBRARY HARRY POTTER ❯*"
}

func handleOpenlibrary(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, openlibraryGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING BOOK....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Docs []struct {
				Title            string   `json:"title"`
				AuthorName       []string `json:"author_name"`
				FirstPublishYear int      `json:"first_publish_year"`
				EditionCount     int      `json:"edition_count"`
				CoverI           int      `json:"cover_i"`
			} `json:"docs"`
		}
		u := "https://openlibrary.org/search.json?limit=1&q=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, u, &res); err != nil || len(res.Docs) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO BOOK FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		d := res.Docs[0]
		var b strings.Builder
		b.WriteString("*🔰 BOOK INFO 🔰*\n\n")
		b.WriteString("*📚 TITLE ❯ " + strings.ToUpper(d.Title) + "*\n")
		if len(d.AuthorName) > 0 {
			b.WriteString("*✍️ AUTHOR ❯ " + strings.ToUpper(strings.Join(d.AuthorName, ", ")) + "*\n")
		}
		if d.FirstPublishYear > 0 {
			b.WriteString("*📅 FIRST PUBLISHED ❯ " + strconv.Itoa(d.FirstPublishYear) + "*\n")
		}
		b.WriteString("*📖 EDITIONS ❯ " + strconv.Itoa(d.EditionCount) + "*")
		if d.CoverI > 0 {
			imgURL := "https://covers.openlibrary.org/b/id/" + strconv.Itoa(d.CoverI) + "-L.jpg"
			if data, err := funGetBytes(ctx, imgURL); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .JIKAN ──────────────────────────────────────────────────────────────────

func jikanGuide(prefix string) string {
	return "*🔰 ANIME INFO 🔰*\n\n" +
		"*GET INFORMATION ABOUT ANY ANIME*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "JIKAN <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "JIKAN NARUTO ❯*"
}

func handleJikan(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, jikanGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING ANIME....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Data struct {
				Media struct {
					Title struct {
						Romaji string `json:"romaji"`
					} `json:"title"`
					AverageScore int    `json:"averageScore"`
					Episodes     int    `json:"episodes"`
					Status       string `json:"status"`
					Description  string `json:"description"`
					CoverImage   struct {
						Large string `json:"large"`
					} `json:"coverImage"`
				} `json:"Media"`
			} `json:"data"`
		}
		body := []byte(`{"query":"{Media(search:\"` + strings.ReplaceAll(q, `"`, "") + `\",type:ANIME){title{romaji} averageScore episodes status description coverImage{large}}}"}`)
		err := funPostJSON(ctx, "https://graphql.anilist.co", body, &res)
		if err != nil || res.Data.Media.Title.Romaji == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO ANIME FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		a := res.Data.Media
		syn := stripHTMLTags(a.Description)
		if len(syn) > 500 {
			syn = syn[:500] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 ANIME INFO 🔰*\n\n")
		b.WriteString("*🎌 TITLE ❯ " + strings.ToUpper(a.Title.Romaji) + "*\n")
		if a.AverageScore > 0 {
			b.WriteString("*⭐ SCORE ❯ " + fmt.Sprintf("%.2f/10", float64(a.AverageScore)/10.0) + "*\n")
		}
		if a.Episodes > 0 {
			b.WriteString("*📺 EPISODES ❯ " + strconv.Itoa(a.Episodes) + "*\n")
		}
		if a.Status != "" {
			b.WriteString("*📌 STATUS ❯ " + strings.ToUpper(a.Status) + "*\n")
		}
		if syn != "" {
			b.WriteString("\n*📖 " + syn + "*")
		}
		if a.CoverImage.Large != "" {
			if data, err := funGetBytes(ctx, a.CoverImage.Large); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// stripHTMLTags removes simple HTML tags and decodes a few common entities.
func stripHTMLTags(in string) string {
	if in == "" {
		return ""
	}
	var out strings.Builder
	depth := 0
	for _, r := range in {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	res := out.String()
	res = strings.ReplaceAll(res, "&quot;", "\"")
	res = strings.ReplaceAll(res, "&#039;", "'")
	res = strings.ReplaceAll(res, "&amp;", "&")
	res = strings.ReplaceAll(res, "&lt;", "<")
	res = strings.ReplaceAll(res, "&gt;", ">")
	res = strings.ReplaceAll(res, "\n", " ")
	res = strings.Join(strings.Fields(res), " ")
	return res
}

func githubuserGuide(prefix string) string {
	return "*🔰 GITHUB PROFILE 🔰*\n\n" +
		"*GET A GITHUB USER PROFILE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "GITHUBUSER <USERNAME> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "GITHUBUSER TORVALDS ❯*"
}

func handleGithubuser(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		user := strings.TrimSpace(strings.Join(args, ""))
		if user == "" {
			s.Reply(info, githubuserGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PROFILE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Login     string `json:"login"`
			Name      string `json:"name"`
			Bio       string `json:"bio"`
			Company   string `json:"company"`
			Location  string `json:"location"`
			Blog      string `json:"blog"`
			Repos     int    `json:"public_repos"`
			Followers int    `json:"followers"`
			Following int    `json:"following"`
			AvatarURL string `json:"avatar_url"`
		}
		u := "https://api.github.com/users/" + url.QueryEscape(user)
		if err := funGetJSON(ctx, u, &res); err != nil || res.Login == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 USER NOT FOUND, PLEASE CHECK THE USERNAME*")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 GITHUB PROFILE 🔰*\n\n")
		b.WriteString("*👤 USERNAME ❯ " + strings.ToUpper(res.Login) + "*\n")
		if res.Name != "" {
			b.WriteString("*📛 NAME ❯ " + strings.ToUpper(res.Name) + "*\n")
		}
		if res.Bio != "" {
			b.WriteString("*📝 BIO ❯ " + res.Bio + "*\n")
		}
		if res.Company != "" {
			b.WriteString("*🏢 COMPANY ❯ " + strings.ToUpper(res.Company) + "*\n")
		}
		if res.Location != "" {
			b.WriteString("*📍 LOCATION ❯ " + strings.ToUpper(res.Location) + "*\n")
		}
		b.WriteString("*📦 REPOS ❯ " + strconv.Itoa(res.Repos) + "*\n")
		b.WriteString("*👥 FOLLOWERS ❯ " + strconv.Itoa(res.Followers) + "*\n")
		b.WriteString("*➡️ FOLLOWING ❯ " + strconv.Itoa(res.Following) + "*")
		if res.AvatarURL != "" {
			if data, err := funGetBytes(ctx, res.AvatarURL); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .OFFICIALJOKE ───────────────────────────────────────────────────────────

func officialjokeGuide(prefix string) string {
	return "*🔰 RANDOM JOKE 🔰*\n\n" +
		"*GET A RANDOM JOKE WITH PUNCHLINE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "OFFICIALJOKE ❯*"
}

func handleOfficialjoke(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING JOKE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Type      string `json:"type"`
			Setup     string `json:"setup"`
			Punchline string `json:"punchline"`
		}
		if err := funGetJSON(ctx, "https://official-joke-api.appspot.com/random_joke", &res); err != nil || res.Setup == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "OFFICIALJOKE")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 RANDOM JOKE 🔰*\n\n")
		b.WriteString("*🏷️ TYPE ❯ " + strings.ToUpper(res.Type) + "*\n\n")
		b.WriteString("*" + res.Setup + "*\n\n")
		b.WriteString("*😂 " + res.Punchline + "*")
		s.Reply(info, b.String())
	})
}

// ── .PYPIINFO ───────────────────────────────────────────────────────────────

func pypiinfoGuide(prefix string) string {
	return "*🔰 PYPI PACKAGE 🔰*\n\n" +
		"*GET INFORMATION ABOUT A PYPI PACKAGE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "PYPIINFO <PACKAGE> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "PYPIINFO REQUESTS ❯*"
}

func handlePypiinfo(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		pkg := strings.TrimSpace(strings.Join(args, ""))
		if pkg == "" {
			s.Reply(info, pypiinfoGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING PACKAGE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Info struct {
				Name     string `json:"name"`
				Version  string `json:"version"`
				Author   string `json:"author"`
				Summary  string `json:"summary"`
				HomePage string `json:"home_page"`
				License  string `json:"license"`
			} `json:"info"`
		}
		u := "https://pypi.org/pypi/" + url.QueryEscape(pkg) + "/json"
		if err := funGetJSON(ctx, u, &res); err != nil || res.Info.Name == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 PACKAGE NOT FOUND, PLEASE CHECK THE NAME*")
			}
			return
		}
		i := res.Info
		var b strings.Builder
		b.WriteString("*🔰 PYPI PACKAGE 🔰*\n\n")
		b.WriteString("*📦 NAME ❯ " + strings.ToUpper(i.Name) + "*\n")
		b.WriteString("*🏷️ VERSION ❯ " + i.Version + "*\n")
		if i.Author != "" {
			b.WriteString("*✍️ AUTHOR ❯ " + strings.ToUpper(i.Author) + "*\n")
		}
		if i.Summary != "" {
			b.WriteString("*📝 SUMMARY ❯ " + i.Summary + "*\n")
		}
		if i.License != "" {
			b.WriteString("*⚖️ LICENSE ❯ " + strings.ToUpper(i.License) + "*")
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .AYAH ───────────────────────────────────────────────────────────────────

func ayahGuide(prefix string) string {
	return "*\U0001f530 QURAN AYAH \U0001f530*\n\n" +
		"*GET ANY QURAN AYAH BY NUMBER (ARABIC + ENGLISH)*\n\n" +
		"*HOW TO USE:*\n" +
		"*\u276e " + prefix + "AYAH <NUMBER> \u276f*\n" +
		"*EXAMPLE \u276e " + prefix + "AYAH 262 \u276f*"
}

func handleAyah(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		num := strings.TrimSpace(strings.Join(args, ""))
		if num == "" {
			s.Reply(info, ayahGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*FETCHING AYAH....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()

		// Fetch BOTH the Arabic (Uthmani) text and the English translation.
		var res struct {
			Code int `json:"code"`
			Data []struct {
				NumberInSurah int    `json:"numberInSurah"`
				Text          string `json:"text"`
				Edition       struct {
					Identifier string `json:"identifier"`
				} `json:"edition"`
				Surah struct {
					EnglishName string `json:"englishName"`
					Number      int    `json:"number"`
				} `json:"surah"`
			} `json:"data"`
		}
		u := "https://api.alquran.cloud/v1/ayah/" + url.QueryEscape(num) + "/editions/quran-uthmani,en.asad"
		if err := funGetJSONRetry(ctx, u, &res, 2); err != nil || res.Code != 200 || len(res.Data) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*\U0001f530 AYAH NOT FOUND, PLEASE CHECK THE NUMBER*")
			}
			return
		}
		var arabic, english string
		var surahName string
		var surahNum, ayahNum int
		for _, d := range res.Data {
			if surahName == "" {
				surahName = d.Surah.EnglishName
				surahNum = d.Surah.Number
				ayahNum = d.NumberInSurah
			}
			if strings.Contains(d.Edition.Identifier, "quran-uthmani") {
				arabic = d.Text
			} else {
				english = d.Text
			}
		}
		var b strings.Builder
		b.WriteString("*\U0001f530 QURAN AYAH \U0001f530*\n\n")
		b.WriteString("*\U0001f4d6 SURAH \u276f " + strings.ToUpper(surahName) + " (" + strconv.Itoa(surahNum) + ")*\n")
		b.WriteString("*\U0001f522 AYAH \u276f " + strconv.Itoa(ayahNum) + "*\n\n")
		if arabic != "" {
			b.WriteString("*\U0001f54b ARABIC:*\n" + arabic + "\n\n")
		}
		if english != "" {
			b.WriteString("*\U0001f1ec\U0001f1e7 ENGLISH:*\n" + english)
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .WIKISEARCH ─────────────────────────────────────────────────────────────

func wikisearchGuide(prefix string) string {
	return "*🔰 WIKIPEDIA 🔰*\n\n" +
		"*GET A WIKIPEDIA SUMMARY FOR ANY TOPIC*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "WIKISEARCH <QUERY> ❯*\n" +
		"*EXAMPLE ❮ " + prefix + "WIKISEARCH GO PROGRAMMING ❯*"
}

func handleWikisearch(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		q := strings.TrimSpace(strings.Join(args, " "))
		if q == "" {
			s.Reply(info, wikisearchGuide(prefix))
			return
		}
		waitID := s.ReplyWithID(info, "*SEARCHING WIKIPEDIA....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		// First resolve the best matching article title via the search API.
		var search struct {
			Query struct {
				Search []struct {
					Title string `json:"title"`
				} `json:"search"`
			} `json:"query"`
		}
		su := "https://en.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=1&srsearch=" + url.QueryEscape(q)
		if err := funGetJSON(ctx, su, &search); err != nil || len(search.Query.Search) == 0 {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO ARTICLE FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		title := search.Query.Search[0].Title
		var res struct {
			Title     string `json:"title"`
			Extract   string `json:"extract"`
			Thumbnail struct {
				Source string `json:"source"`
			} `json:"thumbnail"`
		}
		u := "https://en.wikipedia.org/api/rest_v1/page/summary/" + url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
		if err := funGetJSON(ctx, u, &res); err != nil || res.Extract == "" {
			if !ctxTimedOut(ctx) {
				s.Reply(info, "*🔰 NO ARTICLE FOUND, PLEASE TRY ANOTHER QUERY*")
			}
			return
		}
		ext := res.Extract
		if len(ext) > 900 {
			ext = ext[:900] + "..."
		}
		var b strings.Builder
		b.WriteString("*🔰 WIKIPEDIA 🔰*\n\n")
		b.WriteString("*📚 " + strings.ToUpper(res.Title) + "*\n\n")
		b.WriteString("*" + ext + "*")
		if res.Thumbnail.Source != "" {
			if data, err := funGetBytes(ctx, res.Thumbnail.Source); err == nil {
				_ = s.SendImage(info, data, strings.TrimSpace(b.String()))
				return
			}
		}
		s.Reply(info, strings.TrimSpace(b.String()))
	})
}

// ── .LINKPREVIEW ────────────────────────────────────────────────────────────

func linkpreviewGuide(prefix string) string {
	return "*\U0001f530 LINK PREVIEW \U0001f530*\n\n" +
		"*GET A PREVIEW + SCREENSHOT OF ANY WEBSITE LINK*\n\n" +
		"*HOW TO USE:*\n" +
		"*\u276e " + prefix + "LINKPREVIEW <URL> \u276f*\n" +
		"*EXAMPLE \u276e " + prefix + "LINKPREVIEW HTTPS://EXAMPLE.COM \u276f*"
}

func handleLinkpreview(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		link := strings.TrimSpace(strings.Join(args, ""))
		if link == "" {
			s.Reply(info, linkpreviewGuide(prefix))
			return
		}
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			link = "https://" + link
		}
		waitID := s.ReplyWithID(info, "*FETCHING PREVIEW....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Status string `json:"status"`
			Data   struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Publisher   string `json:"publisher"`
				Author      string `json:"author"`
				URL         string `json:"url"`
				Image       struct {
					URL string `json:"url"`
				} `json:"image"`
				Screenshot struct {
					URL string `json:"url"`
				} `json:"screenshot"`
			} `json:"data"`
		}
		// Request a live screenshot of the page in addition to the metadata.
		u := "https://api.microlink.io/?url=" + url.QueryEscape(link) + "&screenshot=true"
		if err := funGetJSONRetry(ctx, u, &res, 2); err != nil || res.Status != "success" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "LINKPREVIEW")
			}
			return
		}
		d := res.Data
		var b strings.Builder
		b.WriteString("*\U0001f530 LINK PREVIEW \U0001f530*\n\n")
		if d.Title != "" {
			b.WriteString("*\U0001f4f0 TITLE \u276f " + strings.ToUpper(d.Title) + "*\n")
		}
		if d.Publisher != "" {
			b.WriteString("*\U0001f3e2 PUBLISHER \u276f " + strings.ToUpper(d.Publisher) + "*\n")
		}
		if d.Author != "" {
			b.WriteString("*\u270d\ufe0f AUTHOR \u276f " + strings.ToUpper(d.Author) + "*\n")
		}
		if d.Description != "" {
			desc := d.Description
			if len(desc) > 400 {
				desc = desc[:400] + "..."
			}
			b.WriteString("*\U0001f4dd " + desc + "*\n")
		}
		b.WriteString("*\U0001f517 " + d.URL + "*")
		caption := strings.TrimSpace(b.String())

		// Prefer the live screenshot; fall back to the page's og:image.
		shotURL := d.Screenshot.URL
		if shotURL == "" {
			shotURL = d.Image.URL
		}
		if shotURL != "" {
			if data, err := funGetBytes(ctx, shotURL); err == nil {
				_ = s.SendImage(info, data, caption)
				return
			}
		}
		s.Reply(info, caption)
	})
}

// ── .KANYEREST ──────────────────────────────────────────────────────────────

func kanyerestGuide(prefix string) string {
	return "*🔰 KANYE QUOTE 🔰*\n\n" +
		"*GET A RANDOM KANYE WEST QUOTE*\n\n" +
		"*HOW TO USE:*\n" +
		"*❮ " + prefix + "KANYEREST ❯*"
}

func handleKanyerest(s SessionBridge, info types.MessageInfo, args []string, prefix string) {
	RunWithTimeout(s, info, func(ctx context.Context) {
		waitID := s.ReplyWithID(info, "*FETCHING QUOTE....*")
		defer func() { _ = s.DeleteMessage(info, waitID) }()
		var res struct {
			Quote string `json:"quote"`
		}
		if err := funGetJSON(ctx, "https://api.kanye.rest/", &res); err != nil || res.Quote == "" {
			if !ctxTimedOut(ctx) {
				funFail(s, info, "KANYEREST")
			}
			return
		}
		var b strings.Builder
		b.WriteString("*🔰 KANYE QUOTE 🔰*\n\n")
		b.WriteString("*\"" + res.Quote + "\"*")
		s.Reply(info, b.String())
	})
}

func init() {
	Register(Command{Name: "tvmaze", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFORMATION ABOUT ANY TV SHOW. USE IT AS .TVMAZE <QUERY>.", Run: handleTvmaze})
	Register(Command{Name: "openlibrary", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFORMATION ABOUT ANY BOOK. USE IT AS .OPENLIBRARY <QUERY>.", Run: handleOpenlibrary})
	Register(Command{Name: "jikan", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFORMATION ABOUT ANY ANIME. USE IT AS .JIKAN <QUERY>.", Run: handleJikan})
	Register(Command{Name: "githubuser", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A GITHUB USER PROFILE. USE IT AS .GITHUBUSER <USERNAME>.", Run: handleGithubuser})
	Register(Command{Name: "officialjoke", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM JOKE WITH PUNCHLINE. USE IT AS .OFFICIALJOKE.", Run: handleOfficialjoke})
	Register(Command{Name: "pypiinfo", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET INFORMATION ABOUT A PYPI PACKAGE. USE IT AS .PYPIINFO <PACKAGE>.", Run: handlePypiinfo})
	Register(Command{Name: "ayah", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET ANY QURAN AYAH BY NUMBER. USE IT AS .AYAH <NUMBER>.", Run: handleAyah})
	Register(Command{Name: "wikisearch", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A WIKIPEDIA SUMMARY FOR ANY TOPIC. USE IT AS .WIKISEARCH <QUERY>.", Run: handleWikisearch})
	Register(Command{Name: "linkpreview", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A PREVIEW OF ANY WEBSITE LINK. USE IT AS .LINKPREVIEW <URL>.", Run: handleLinkpreview})
	Register(Command{Name: "kanyerest", Category: "TOOLS", Desc: "THIS COMMAND IS USED TO GET A RANDOM KANYE WEST QUOTE. USE IT AS .KANYEREST.", Run: handleKanyerest})

	// hidden aliases
	Register(Command{Name: "tvshowinfo", Category: "TOOLS", Desc: "Short alias of .tvmaze", Hidden: true, Run: handleTvmaze})
	Register(Command{Name: "bookinfo", Category: "TOOLS", Desc: "Short alias of .openlibrary", Hidden: true, Run: handleOpenlibrary})
	Register(Command{Name: "animeinfo", Category: "TOOLS", Desc: "Short alias of .jikan", Hidden: true, Run: handleJikan})
	Register(Command{Name: "ghuser", Category: "TOOLS", Desc: "Short alias of .githubuser", Hidden: true, Run: handleGithubuser})
	Register(Command{Name: "jokepunch", Category: "TOOLS", Desc: "Short alias of .officialjoke", Hidden: true, Run: handleOfficialjoke})
	Register(Command{Name: "pypipkg", Category: "TOOLS", Desc: "Short alias of .pypiinfo", Hidden: true, Run: handlePypiinfo})
	Register(Command{Name: "ayahinfo", Category: "TOOLS", Desc: "Short alias of .ayah", Hidden: true, Run: handleAyah})
	Register(Command{Name: "wikisummary", Category: "TOOLS", Desc: "Short alias of .wikisearch", Hidden: true, Run: handleWikisearch})
	Register(Command{Name: "linkinfo", Category: "TOOLS", Desc: "Short alias of .linkpreview", Hidden: true, Run: handleLinkpreview})
	Register(Command{Name: "kanyequote", Category: "TOOLS", Desc: "Short alias of .kanyerest", Hidden: true, Run: handleKanyerest})
}
