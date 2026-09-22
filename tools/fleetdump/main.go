// fleetdump v3: Storj fleet-state prober — prefix list (real HGETALL view).
package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// EXACT mirror of hardcodedStorjShards (storage.go) — STORADERA migration
// (2026-09-16). All 10 slots = same creds → FNV-1a sharding converges on
// ONE bucket "goldmd". Purane Storj umar/umar2..10 sets expired/deleted.
var shards = [10][3]string{
	{"AKIA6139I7AGJR0L0630", "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz", "goldmd"},
	{"AKIA3380AVOH90WMPB69", "Pm3P6PbtCNyMDdczbaSzefrzp0K5gvcqfa8E1xKq", "gold2"},
	{"AKIA35754JXDYFQK7XHR", "R95egkdTXmE5K6UO876vzPGz9jBMq7y0FKpKX61K", "gold3"},
	{"AKIA2083VJSVVU88VCEB", "yRus8xhVGEPW0Efyv0nhhRR40hfGgK173ZkIzjlS", "gold4"},
	{"AKIA90170CO4BWF3OWUG", "aUDHw1vDI7d7zOg73uGhztN7lfvNb84C73mCoasL", "gold5"},
	{"AKIA6111E0URFD3575BN", "Larcnkizs7gk9sGpAUPcgGUEVOjOVjlaM70jkwE5", "gold6"},
	{"AKIA24138MTI05TQ6MTD", "5Aui0Yc6rdLMv6hOZt7rOyIEXlGYrUaAKXaW4faX", "gold7"},
	{"AKIA1094TFMGV9M200IT", "pyYe6Hz4X8STOmV4i6ZoJFeASExwWlwqyYIR2Mh3", "gold8"},
	{"AKIA3668SXGPLQA0HUJ0", "ahnlVCSEDC1EKx2jd3b31xt6rFtBBXgOYLAPdcT8", "gold9"},
	{"AKIA6658WW37WXCBFG0Q", "1yP3vhEC6UU8hxp3DlZskmqAYnTeuZ1T9gXfgHi9", "gold10"},
}

func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
func shardFor(key string) int { return int(fnv32(key) % 10) }
func kvE(s string) string     { return url.PathEscape(s) }

func newCli(sh [3]string) (*minio.Client, error) {
	return minio.New("eu-east-1.s3.storadera.com", &minio.Options{
		Creds: credentials.NewStaticV4(sh[0], sh[1], ""), Region: "eu-east-1", Secure: true,
	})
}

func main() {
	now := time.Now().Unix()

	// 1) servers hash — list ALL fields under prefix (HGETALL view)
	k := "goldmd:fleet:servers"
	sh := shards[shardFor(k)]
	cli, err := newCli(sh)
	if err != nil {
		fmt.Println("ERR client:", err)
		return
	}
	fmt.Printf("=== %s on bucket %s ===\n", k, sh[2])
	objCh := cli.ListObjects(context.Background(), sh[2], minio.ListObjectsOptions{
		Prefix: "kv/hashes/" + kvE(k) + "/", Recursive: true,
	})
	n := 0
	for obj := range objCh {
		if obj.Err != nil {
			fmt.Println("  list err:", obj.Err)
			break
		}
		n++
		field := strings.TrimPrefix(obj.Key, "kv/hashes/"+kvE(k)+"/")
		field, _ = url.PathUnescape(field)
		// read value
		o, err := cli.GetObject(context.Background(), sh[2], obj.Key, minio.GetObjectOptions{})
		if err != nil {
			continue
		}
		buf := make([]byte, 256)
		m := 0
		for m < len(buf) {
			r, err := o.Read(buf[m:])
			m += r
			if err != nil {
				break
			}
		}
		o.Close()
		val := strings.TrimSpace(string(buf[:m]))
		ts := int64(0)
		if seg := strings.SplitN(val, "|", 2); len(seg) > 0 {
			fmt.Sscanf(seg[0], "%d", &ts)
		}
		age := now - ts
		status := "DEAD"
		if age >= 0 && age < 120 {
			status = "FRESH"
		} else if age >= 0 && age < 600 {
			status = "recent"
		} else if age < 0 {
			status = "future?"
		}
		fmt.Printf("  %-45s %-8s age=%ds val=%s\n", field, status, age, trunc(val, 60))
	}
	fmt.Printf("  (total fields: %d)\n", n)

	// 2) fleet session registry set
	for _, setKey := range []string{"goldmd:fleet:sessions", "goldmd:sessionjids:svr11221", "goldmd:sessionjids:svr11221:members"} {
		sh2 := shards[shardFor(setKey)]
		cli2, _ := newCli(sh2)
		found := false
		objCh2 := cli2.ListObjects(context.Background(), sh2[2], minio.ListObjectsOptions{
			Prefix: "kv/sets/" + kvE(setKey) + "/", Recursive: true,
		})
		cnt := 0
		for obj := range objCh2 {
			if obj.Err != nil {
				break
			}
			found = true
			cnt++
			member, _ := url.PathUnescape(strings.TrimPrefix(obj.Key, "kv/sets/"+kvE(setKey)+"/"))
			fmt.Printf("SET %-40s member: %s\n", setKey, member)
		}
		if !found {
			fmt.Printf("SET %-40s => (empty/missing)\n", setKey)
		} else if cnt == 0 {
			fmt.Printf("SET %-40s => (prefix dir, %d listed)\n", setKey, cnt)
		}
	}

	// 3) per-JID fleet keys for the deleted session
	jid := "923158930864"
	for _, jk := range []string{
		"goldmd:fleet:sess:" + jid,
		"goldmd:fleet:dispatch:" + jid,
		"goldmd:fleet:claim:" + jid,
		"goldmd:fleet:meta:" + jid,
		"goldmd:fleet:fail:" + jid,
	} {
		sh3 := shards[shardFor(jk)]
		cli3, _ := newCli(sh3)
		for _, pfx := range []string{"kv/strings/", "kv/hashes/"} {
			o, err := cli3.GetObject(context.Background(), sh3[2], pfx+kvE(jk), minio.GetObjectOptions{})
			if err != nil {
				continue
			}
			buf := make([]byte, 4096)
			m, last := 0, 0
			for m < len(buf) {
				r, err := o.Read(buf[m:])
				m += r
				if err != nil {
					if r > 0 {
						last = m
					}
					break
				}
				last = m
			}
			o.Close()
			v := strings.TrimSpace(string(buf[:last]))
			if v != "" {
				fmt.Printf("JID %-45s %-13s => %s\n", jk, pfx, trunc(v, 80))
			}
		}
	}
	// claim hash fields
	ck := "goldmd:fleet:claim:" + jid
	sh4 := shards[shardFor(ck)]
	cli4, _ := newCli(sh4)
	objCh4 := cli4.ListObjects(context.Background(), sh4[2], minio.ListObjectsOptions{
		Prefix: "kv/hashes/" + kvE(ck) + "/", Recursive: true,
	})
	for obj := range objCh4 {
		if obj.Err != nil {
			break
		}
		field, _ := url.PathUnescape(strings.TrimPrefix(obj.Key, "kv/hashes/"+kvE(ck)+"/"))
		o, _ := cli4.GetObject(context.Background(), sh4[2], obj.Key, minio.GetObjectOptions{})
		buf := make([]byte, 128)
		m := 0
		for m < len(buf) {
			r, err := o.Read(buf[m:])
			m += r
			if err != nil {
				break
			}
		}
		o.Close()
		fmt.Printf("CLAIM %s field=%s val=%s\n", jid, field, strings.TrimSpace(string(buf[:m])))
	}
	fmt.Println("DONE")
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
