package goldcmds

import (
	"context"
	"testing"
	"time"
)

// TestMetaMultiLive checks the metadata pipeline across several video types
// (normal, music, shorts, live) to find which ones resolve to N/A.
// It now also exercises yt2FetchDetails (ANDROID_TESTSUITE) which must
// return author + duration even for UNPLAYABLE / LOGIN_REQUIRED videos.
func TestMetaMultiLive(t *testing.T) {
	vids := []string{
		"dQw4w9WgXcQ", // normal music video
		"kJQP7kiw5Fk", // Despacito (music)
		"9bZkp7q19f0", // Gangnam Style
		"JGwWNGJdvx8", // Ed Sheeran Shape of You
		"OPf0YbXqDm0", // Mark Ronson Uptown Funk
		"hT_nvWreIhg", // OneRepublic Counting Stars
	}
	for _, vid := range vids {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var meta *yt2Meta
		var details *yt2Details
		done := make(chan struct{})
		go func() {
			defer close(done)
			meta = yt2FetchMeta(ctx, vid)
			details = yt2FetchDetails(ctx, vid)
		}()
		st := yt2RaceFetch(ctx, vid, false)
		<-done
		cancel()

		raceAuthor, raceDur := "", ""
		if st != nil {
			raceAuthor, raceDur = st.author, st.duration
		}
		metaAuthor := ""
		if meta != nil {
			metaAuthor = meta.author
		}
		detAuthor, detDur := "", ""
		if details != nil {
			detAuthor, detDur = details.author, details.duration
		}

		// FINAL resolution mirrors the patched pipelines: details first.
		author := detAuthor
		if author == "" {
			author = metaAuthor
		}
		if author == "" {
			author = raceAuthor
		}
		if author == "" {
			author = "N/A"
		}
		dur := raceDur
		if dur == "" {
			dur = detDur
		}
		if dur == "" {
			dur = "N/A"
		}
		t.Logf("%s | race=%v raceAuthor=%q raceDur=%q | metaAuthor=%q | detAuthor=%q detDur=%q | FINAL author=%q dur=%q",
			vid, st != nil, raceAuthor, raceDur, metaAuthor, detAuthor, detDur, author, dur)
		if author == "N/A" || dur == "N/A" {
			t.Errorf("%s STILL N/A: author=%q dur=%q", vid, author, dur)
		}
	}
}
