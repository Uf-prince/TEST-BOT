package main

// TEMPORARY probe — live view of the failover-test bucket fleet KV state.
// Run with: GOLDMD_TEST_BUCKET_PROBE=1 go test -run TestStorjFleetKVProbe -count=1 -v .

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestStorjFleetKVProbe(t *testing.T) {
	if os.Getenv("GOLDMD_TEST_BUCKET_PROBE") != "1" {
		t.Skip("probe disabled — set GOLDMD_TEST_BUCKET_PROBE=1")
	}
	const testBucket = "goldmd-failover-test"
	// probeTargetSID: jis server ke per-sid data ki purge verify karni hai
	// (GOLDMD_PROBE_TARGET_SID env se override kar sakte ho).
	probeTargetSID := os.Getenv("GOLDMD_PROBE_TARGET_SID")
	if probeTargetSID == "" {
		probeTargetSID = "https://hash-scenarios-heather-pros.trycloudflare.com"
	}
	for i := 1; i <= 10; i++ {
		_ = os.Setenv("STORJ_ACCESS_KEY_"+strconv.Itoa(i), hardcodedStorjShards[0][0])
		_ = os.Setenv("STORJ_SECRET_KEY_"+strconv.Itoa(i), hardcodedStorjShards[0][1])
		_ = os.Setenv("STORJ_BUCKET_"+strconv.Itoa(i), testBucket)
	}
	if err := InitStorj(); err != nil {
		t.Fatalf("InitStorj failed: %v", err)
	}
	ctx := context.Background()
	now := time.Now().Unix()

	hbKey := "goldmd:fleet:servers"
	shard := storj.shardForID(hbKey)
	prefix := kvHashesPrefix + kvEncode(hbKey) + "/"
	t.Logf("== HEARTBEATS ==")
	found := 0
	for obj := range shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			continue
		}
		field := kvDecode(trimObjPrefix(obj.Key, prefix))
		data, ok, err := kvReadBytesProbe(ctx, shard, obj.Key)
		if err != nil || !ok {
			continue
		}
		ts, _ := strconv.ParseInt(string(data), 10, 64)
		age := now - ts
		found++
		t.Logf("  sid=%q ts=%d age=%ds %s", field, ts, age, aliveTag(age))
	}
	if found == 0 {
		t.Logf("  (no heartbeats yet)")
	}

	t.Logf("== CLAIM HASHES (fleet) ==")
	for obj := range shard.client.ListObjects(ctx, shard.bucket, minio.ListObjectsOptions{Prefix: kvHashesPrefix + kvEncode("goldmd:fleet:claim:"), Recursive: true}) {
		if obj.Err != nil {
			continue
		}
		data, ok, err := kvReadBytesProbe(ctx, shard, obj.Key)
		if err != nil || !ok {
			continue
		}
		ts, _ := strconv.ParseInt(string(data), 10, 64)
		t.Logf("  claim=%q ts=%d age=%ds", obj.Key, ts, now-ts)
	}

	t.Logf("== SESSIONDB KEYS (per-sid blob/jids — purge check) ==")
	{
		// dead server ke per-sid data delete hue ya nahi (blob + jids)
		for _, key := range []string{
			"goldmd:sessiondb:" + probeTargetSID + ":blob",
			"goldmd:sessiondb:" + probeTargetSID + ":jids",
			"goldmd:sessiondb:svr1:blob",   // legacy shared key — must NOT return
			"goldmd:sessiondb:svr1:jids",   // legacy shared key — must NOT return
		} {
			sh := storj.shardForID(key)
			data, ok, err := kvReadBytesProbe(ctx, sh, kvStringsPrefix+kvEncode(key))
			if err == nil && ok {
				t.Logf("  FOUND %-70q len=%d", key, len(data))
			} else {
				t.Logf("  GONE  %-70q (deleted / never existed)", key)
			}
		}
	}

	t.Logf("== FAIL MARKERS ==")
	n4 := 0
	for i := 1; i <= 10; i++ {
		sh := storj.shards[i-1]
		for obj := range sh.client.ListObjects(ctx, sh.bucket, minio.ListObjectsOptions{Prefix: kvStringsPrefix + kvEncode("goldmd:fleet:fail:"), Recursive: true}) {
			if obj.Err != nil {
				continue
			}
			data, ok, err := kvReadBytesProbe(ctx, sh, obj.Key)
			if err != nil || !ok {
				continue
			}
			t.Logf("  marker=%q dead-sid=%q", obj.Key, string(data))
			n4++
		}
	}
	if n4 == 0 {
		t.Logf("  (none — marker DEL = notification FIRED ✓)")
	}
}

func trimObjPrefix(k, p string) string {
	if len(k) > len(p) {
		return k[len(p):]
	}
	return k
}

func kvReadBytesProbe(ctx context.Context, shard *storjShard, key string) ([]byte, bool, error) {
	obj, err := shard.client.GetObject(ctx, shard.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, false, err
	}
	defer obj.Close()
	buf := make([]byte, 4096)
	n, err := obj.Read(buf)
	if err != nil && n == 0 {
		return nil, false, err
	}
	return buf[:n], true, nil
}

func aliveTag(age int64) string {
	if age < 120 {
		return "FRESH-ALIVE"
	}
	return "STALE"
}
