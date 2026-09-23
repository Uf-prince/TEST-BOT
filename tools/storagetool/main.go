package main

// ============================================================================
// storagetool — GOLD-MD Storadera (S3) inspector / editor
// ----------------------------------------------------------------------------
// Modes:
//   get <bucket> <key>            → print the raw value of an object
//   search <needle>               → list every object across all 10 buckets
//                                   whose key contains <needle>
//   getprefix <jid>               → read prefix:<jid> (the bot's command prefix)
//   setprefix <jid> <value>       → write prefix:<jid> + add jid to prefix:keys
//   delprefix <jid>               → remove prefix:<jid> (falls back to default)
//
// The KV layout mirrors src/storage.go:
//   kv/strings/<url-escaped key>          → plain string value
//   kv/sets/<url-escaped key>/<member>    → one object per set member
// Sharding: FNV-1a 32-bit of the LOGICAL key, % 10 → bucket index
//   (0=goldmd, 1=gold2, ... 9=gold10) — identical to shardForID().
// ============================================================================

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var creds2 = map[string][2]string{
	"goldmd": {"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz"},
	"gold2":  {"AKIA3380AVOH90WMPB69", "Pm3P6PbtCNyMDdczbaSzefrzp0K5gvcqfa8E1xKq"},
	"gold3":  {"AKIA35754JXDYFQK7XHR", "R95egkdTXmE5K6UO876vzPGz9jBMq7y0FKpKX61K"},
	"gold4":  {"AKIA2083VJSVVU88VCEB", "yRus8xhVGEPW0Efyv0nhhRR40hfGgK173ZkIzjlS"},
	"gold5":  {"AKIA90170CO4BWF3OWUG", "aUDHw1vDI7d7zOg73uGhztN7lfvNb84C73mCoasL"},
	"gold6":  {"AKIA6111E0URFD3575BN", "Larcnkizs7gk9sGpAUPcgGUEVOjOVjlaM70jkwE5"},
	"gold7":  {"AKIA24138MTI05TQ6MTD", "5Aui0Yc6rdLMv6hOZt7rOyIEXlGYrUaAKXaW4faX"},
	"gold8":  {"AKIA1094TFMGV9M200IT", "pyYe6Hz4X8STOmV4i6ZoJFeASExwWlwqyYIR2Mh3"},
	"gold9":  {"AKIA3668SXGPLQA0HUJ0", "ahnlVCSEDC1EKx2jd3b31xt6rFtBBXgOYLAPdcT8"},
	"gold10": {"AKIA6658WW37WXCBFG0Q", "1yP3vhEC6UU8hxp3DlZskmqAYnTeuZ1T9gXfgHi9"},
}

// bucketOrder is the shard order used by FNV-1a % 10 (index 0..9).
var bucketOrder = []string{"goldmd", "gold2", "gold3", "gold4", "gold5", "gold6", "gold7", "gold8", "gold9", "gold10"}

func client(bucket string) *minio.Client {
	c := creds2[bucket]
	cli, err := minio.New("eu-east-1.s3.storadera.com", &minio.Options{
		Creds:  credentials.NewStaticV4(c[0], c[1], ""),
		Secure: true,
		Region: "eu-east-1",
	})
	if err != nil {
		log.Fatalf("minio.New: %v", err)
	}
	return cli
}

// shardBucket returns the bucket that owns a logical KV key (FNV-1a % 10),
// exactly like StorjStore.shardForID().
func shardBucket(key string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return bucketOrder[int(h)%len(bucketOrder)]
}

func kvEncode(s string) string { return url.PathEscape(s) }

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: storagetool <get|search|getprefix|setprefix|delprefix> ...")
		return
	}
	mode := os.Args[1]

	switch mode {
	case "get":
		bucket, key := os.Args[2], os.Args[3]
		cli := client(bucket)
		obj, err := cli.GetObject(context.Background(), bucket, key, minio.GetObjectOptions{})
		if err != nil {
			log.Fatalf("get: %v", err)
		}
		b, err := io.ReadAll(obj)
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		fmt.Printf("VALUE[%s/%s] = %q\n", bucket, key, string(b))
		return

	case "search":
		needle := os.Args[2]
		for _, bucket := range bucketOrder {
			cli := client(bucket)
			for obj := range cli.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{Prefix: "", Recursive: true}) {
				if obj.Err != nil {
					continue
				}
				if strings.Contains(obj.Key, needle) {
					fmt.Printf("MATCH %s : %s (%d bytes)\n", bucket, obj.Key, obj.Size)
				}
			}
		}
		return

	case "getprefix":
		jid := os.Args[2]
		key := "prefix:" + jid
		bucket := shardBucket(key)
		cli := client(bucket)
		obj, err := cli.GetObject(context.Background(), bucket, "kv/strings/"+kvEncode(key), minio.GetObjectOptions{})
		if err != nil {
			log.Fatalf("getprefix: %v", err)
		}
		b, err := io.ReadAll(obj)
		if err != nil {
			fmt.Printf("PREFIX[%s] = <not set> (uses default '.')\n", jid)
			return
		}
		fmt.Printf("PREFIX[%s] = %q  (bucket=%s)\n", jid, string(b), bucket)
		return

	case "setprefix":
		jid, val := os.Args[2], os.Args[3]
		key := "prefix:" + jid
		bucket := shardBucket(key)
		cli := client(bucket)
		ctx := context.Background()
		// 1. plain string value
		_, err := cli.PutObject(ctx, bucket, "kv/strings/"+kvEncode(key),
			strings.NewReader(val), int64(len(val)), minio.PutObjectOptions{ContentType: "text/plain"})
		if err != nil {
			log.Fatalf("setprefix string: %v", err)
		}
		// 2. register jid in the prefix:keys set (same shard as the set key)
		setKey := "prefix:keys"
		setBucket := shardBucket(setKey)
		setCli := client(setBucket)
		_, err = setCli.PutObject(ctx, setBucket, "kv/sets/"+kvEncode(setKey)+"/"+kvEncode(jid),
			strings.NewReader("1"), 1, minio.PutObjectOptions{ContentType: "text/plain"})
		if err != nil {
			log.Fatalf("setprefix set: %v", err)
		}
		fmt.Printf("OK  prefix[%s] = %q  (string bucket=%s, set bucket=%s)\n", jid, val, bucket, setBucket)
		return

	case "delprefix":
		jid := os.Args[2]
		key := "prefix:" + jid
		bucket := shardBucket(key)
		cli := client(bucket)
		ctx := context.Background()
		_ = cli.RemoveObject(ctx, bucket, "kv/strings/"+kvEncode(key), minio.RemoveObjectOptions{})
		setKey := "prefix:keys"
		setBucket := shardBucket(setKey)
		setCli := client(setBucket)
		_ = setCli.RemoveObject(ctx, setBucket, "kv/sets/"+kvEncode(setKey)+"/"+kvEncode(jid), minio.RemoveObjectOptions{})
		fmt.Printf("OK  prefix[%s] removed (falls back to default '.')\n", jid)
		return

	default:
		fmt.Println("unknown mode:", mode)
	}
}
