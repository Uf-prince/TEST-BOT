// storadera10: 10 Storadera accounts ka full probe — har account pe
// BucketExists → (nahi to) MakeBucket → PUT → GET → DELETE roundtrip.
// Owner: jani. Bucket connect ho jaye to OK; "not found" aaye to API keys
// se bana do. Output: per-slot verdict + final "10/10 CONNECTED" summary.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	endpoint = "eu-east-1.s3.storadera.com"
	region   = "eu-east-1"
)

type acct struct {
	slot   int
	access string
	secret string
	bucket string
}

var accounts = []acct{
	{1, "AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{2, "AKIA3380AVOH90WMPB69", "Pm3P6PbtCNyMDdczbaSzefrzp0K5gvcqfa8E1xKq", "gold2"},
	{3, "AKIA35754JXDYFQK7XHR", "R95egkdTXmE5K6UO876vzPGz9jBMq7y0FKpKX61K", "Gold3"},
	{4, "AKIA2083VJSVVU88VCEB", "yRus8xhVGEPW0Efyv0nhhRR40hfGgK173ZkIzjlS", "Gold4"},
	{5, "AKIA90170CO4BWF3OWUG", "aUDHw1vDI7d7zOg73uGhztN7lfvNb84C73mCoasL", "Gold5"},
	{6, "AKIA6111E0URFD3575BN", "Larcnkizs7gk9sGpAUPcgGUEVOjOVjlaM70jkwE5", "Gold6"},
	{7, "AKIA24138MTI05TQ6MTD", "5Aui0Yc6rdLMv6hOZt7rOyIEXlGYrUaAKXaW4faX", "Gold7"},
	{8, "AKIA1094TFMGV9M200IT", "pyYe6Hz4X8STOmV4i6ZoJFeASExwWlwqyYIR2Mh3", "Gold8"},
	{9, "AKIA3668SXGPLQA0HUJ0", "ahnlVCSEDC1EKx2jd3b31xt6rFtBBXgOYLAPdcT8", "Gold9"},
	{10, "AKIA6658WW37WXCBFG0Q", "1yP3vhEC6UU8hxp3DlZskmqAYnTeuZ1T9gXfgHi9", "Gold10"},
}

func main() {
	ctx := context.Background()
	okCount := 0
	for _, a := range accounts {
		fmt.Printf("\n=== SLOT %d | bucket=%q | key=%s ===\n", a.slot, a.bucket, a.access)
		cli, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(a.access, a.secret, ""),
			Secure: true,
			Region: region,
		})
		if err != nil {
			fmt.Println("  FAIL client:", err)
			continue
		}
		// bucket name lowercase try (S3 rule) — pehle as-given, phir lower
		bucket := a.bucket
		exists, err := cli.BucketExists(ctx, bucket)
		if err != nil {
			// lowercase retry
			lb := strings.ToLower(bucket)
			if lb != bucket {
				if e2, err2 := cli.BucketExists(ctx, lb); err2 == nil {
					bucket = lb
					exists = e2
					err = nil
					fmt.Printf("  (lowercase bucket use: %q)\n", bucket)
				}
			}
		}
		if err != nil {
			fmt.Println("  FAIL BucketExists:", err)
			continue
		}
		if !exists {
			fmt.Printf("  bucket %q NOT FOUND — MakeBucket...\n", bucket)
			if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
				// lowercase retry
				lb := strings.ToLower(bucket)
				if lb != bucket {
					if err2 := cli.MakeBucket(ctx, lb, minio.MakeBucketOptions{Region: region}); err2 == nil {
						bucket = lb
						fmt.Printf("  MakeBucket OK (lowercase %q)\n", bucket)
					} else {
						fmt.Println("  FAIL MakeBucket:", err, "/", err2)
						continue
					}
				} else {
					fmt.Println("  FAIL MakeBucket:", err)
					continue
				}
			} else {
				fmt.Printf("  MakeBucket OK (%q created)\n", bucket)
			}
		} else {
			fmt.Printf("  BucketExists OK (%q)\n", bucket)
		}
		// PUT
		key := "kv/strings/probe-slot" + fmt.Sprint(a.slot)
		body := []byte("probe-" + time.Now().Format(time.RFC3339Nano))
		_, err = cli.PutObject(ctx, bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "text/plain"})
		if err != nil {
			fmt.Println("  FAIL PUT:", err)
			continue
		}
		// GET
		obj, err := cli.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if err != nil {
			fmt.Println("  FAIL GET:", err)
			continue
		}
		got, _ := io.ReadAll(obj)
		obj.Close()
		if !bytes.Equal(got, body) {
			fmt.Println("  FAIL GET mismatch")
			continue
		}
		// DELETE
		if err := cli.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
			fmt.Println("  FAIL DELETE:", err)
			continue
		}
		fmt.Printf("  ✅ SLOT %d CONNECTED (bucket=%q) PUT/GET/DELETE OK\n", a.slot, bucket)
		okCount++
	}
	fmt.Printf("\n════════════════════════════════════\n%d/10 CONNECTED\n════════════════════════════════════\n", okCount)
}
