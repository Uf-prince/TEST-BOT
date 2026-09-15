package goldcmds

// LIVE DEBUG TEST (temporary) — .tempnumber / .checknumber flow ka exact
// reproduction: tnGetRandomWithInbox se ACTIVE number + inbox verify.
// Run: go test -mod=vendor -v -run TestTempNumberLive ./gold-cmds/ -count=1

import (
	"os"
	"testing"
)

func TestTempNumberLive(t *testing.T) {
	if os.Getenv("TN_LIVE") != "1" {
		t.Skip("set TN_LIVE=1 to run live tempnumber tests")
	}
	num, smsList, err := tnGetRandomWithInbox()
	if err != nil {
		t.Fatalf("tnGetRandomWithInbox FAILED: %v", err)
	}
	t.Logf("ACTIVE NUMBER: +%s | SMS IN INBOX: %d", num, len(smsList))
	if len(smsList) == 0 {
		t.Errorf("GOT EMPTY INBOX — FIX NOT WORKING")
	}
	for i, sms := range smsList {
		if i >= 3 {
			break
		}
		t.Logf("  [%d] FROM=%s OTP=%q TEXT=%.50q", i, sms.From, sms.OTP, sms.Text)
	}
	otpFound := false
	for _, sms := range smsList {
		if sms.OTP != "" {
			otpFound = true
			break
		}
	}
	if !otpFound {
		t.Logf("NOTE: inbox me SMS hai lekin koi OTP-style code nahi (normal hai)")
	} else {
		t.Logf("OTP EXTRACTION WORKING ✅")
	}
}
