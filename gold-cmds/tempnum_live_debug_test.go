package goldcmds

// LIVE DEBUG TEST — .tempnumber / .checknumber flow ka exact
// reproduction: receive-smss.live se country list + LIVE inbox verify.
// Run: TN_LIVE=1 go test -mod=vendor -v -run TestTempNumberLive ./gold-cmds/ -count=1

import (
	"os"
	"testing"
)

func TestTempNumberLive(t *testing.T) {
	if os.Getenv("TN_LIVE") != "1" {
		t.Skip("set TN_LIVE=1 to run live tempnumber tests")
	}
	// India country list fetch
	nums, err := tnCountryNumbers("in")
	if err != nil {
		t.Fatalf("tnCountryNumbers FAILED: %v", err)
	}
	t.Logf("INDIA NUMBERS: %d", len(nums))
	if len(nums) == 0 {
		t.Fatalf("GOT EMPTY COUNTRY LIST — FIX NOT WORKING")
	}
	for i, e := range nums {
		if i >= 5 {
			break
		}
		t.Logf("  [%d] %s | SMS:%s | LAST:%s | APP:%s", i+1, e.Display, e.Messages, e.Fresh, e.App)
	}
	// Sabse fresh number ka LIVE inbox fetch
	list, err := tnGetInbox(nums[0].Number)
	if err != nil {
		t.Fatalf("tnGetInbox FAILED: %v", err)
	}
	t.Logf("INBOX SMS COUNT (num %s): %d", nums[0].Number, len(list))
	if len(list) == 0 {
		t.Errorf("GOT EMPTY INBOX — CHECK INBOX PARSING")
	}
	for i, sms := range list {
		if i >= 3 {
			break
		}
		t.Logf("  [%d] FROM=%s DATE=%s TEXT=%.60q", i, sms.From, sms.Date, sms.Text)
	}
}
