// storaderaprobe: Storadera (eu-east-1.s3.storadera.com) credential + S3 test.
// Stages: CONNECT → LISTBUCKETS → MAKEBUCKET(goldmd) → PUT → GET → DELETE.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const endpoint = "eu-east-1.s3.storadera.com"
const region = "eu-east-1"
const accessKey = "AKIA6139I7AGJR0L0630"
const secretKey = "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz"
const bucket = "goldmd"

func log(stage string, fields map[string]any) {
	out := map[string]any{"stage": stage, "ts": time.Now().Format(time.RFC3339Nano)}
	for k, v := range fields {
		out[k] = v
	}
	fmt.Println(strings.TrimSpace(fmt.Sprintf("JSON %v", out)))
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
		Region: region,
	})
	if err != nil {
		log("CONNECT", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	log("CONNECT", map[string]any{"ok": true, "endpoint": endpoint})

	// LIST BUCKETS
	buckets, err := cli.ListBuckets(ctx)
	if err != nil {
		log("LISTBUCKETS", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	names := []string{}
	for _, b := range buckets {
		names = append(names, b.Name)
	}
	log("LISTBUCKETS", map[string]any{"ok": true, "count": len(names), "buckets": names})

	// ENSURE BUCKET
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		log("BUCKETEXISTS", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !exists {
		if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			log("MAKEBUCKET", map[string]any{"ok": false, "error": err.Error()})
			return
		}
		log("MAKEBUCKET", map[string]any{"ok": true, "bucket": bucket, "created": true})
	} else {
		log("BUCKETEXISTS", map[string]any{"ok": true, "bucket": bucket, "created": false})
	}

	// PUT (kv/hashes emulation — jaise bot storage.go karta hai)
	key := "kv/strings/probe-write-test"
	body := "hello-storadera-" + time.Now().Format("150405")
	_, err = cli.PutObject(ctx, bucket, key, strings.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "text/plain"})
	if err != nil {
		log("PUT", map[string]any{"ok": false, "bucket": bucket, "key": key, "error": err.Error()})
		return
	}
	log("PUT", map[string]any{"ok": true, "bucket": bucket, "key": key, "bytes": len(body)})

	// GET roundtrip
	obj, err := cli.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		log("GET", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	buf := new(strings.Builder)
	tmp := make([]byte, 512)
	for {
		n, rerr := obj.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if rerr != nil {
			break
		}
	}
	obj.Close()
	if buf.String() != body {
		log("GET", map[string]any{"ok": false, "error": "roundtrip-mismatch", "sent": body, "got": buf.String()})
		return
	}
	log("GET", map[string]any{"ok": true, "key": key, "value": buf.String()})

	// LIST prefix (kv/strings/)
	nl := 0
	for obj := range cli.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "kv/", Recursive: true, MaxKeys: 100}) {
		if obj.Err != nil {
			log("LIST", map[string]any{"ok": false, "error": obj.Err.Error()})
			return
		}
		nl++
		if nl <= 5 {
			log("LIST-ITEM", map[string]any{"key": obj.Key, "size": obj.Size})
		}
	}
	log("LIST", map[string]any{"ok": true, "prefix": "kv/", "count": nl})

	// DELETE
	if err := cli.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		log("DELETE", map[string]any{"ok": false, "error": err.Error()})
		return
	}
	log("DELETE", map[string]any{"ok": true, "key": key})
	log("DONE", map[string]any{"ok": true, "verdict": "STORADERA-READY"})
}
