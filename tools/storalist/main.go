package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var creds = map[string][2]string{
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

func main() {
	bucket := "goldmd"
	if len(os.Args) > 1 {
		bucket = os.Args[1]
	}
	prefix := ""
	if len(os.Args) > 2 {
		prefix = os.Args[2]
	}
	c, ok := creds[bucket]
	if !ok {
		log.Fatalf("no creds for bucket %q", bucket)
	}
	cli, err := minio.New("eu-east-1.s3.storadera.com", &minio.Options{
		Creds:  credentials.NewStaticV4(c[0], c[1], ""),
		Secure: true,
		Region: "eu-east-1",
	})
	if err != nil {
		log.Fatalf("minio.New: %v", err)
	}
	ctx := context.Background()
	n := 0
	for obj := range cli.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			log.Fatalf("list err: %v", obj.Err)
		}
		n++
		if n <= 80 {
			fmt.Printf("%10d  %s\n", obj.Size, obj.Key)
		}
	}
	fmt.Printf("--- total objects in %s (prefix=%q): %d ---\n", bucket, prefix, n)
}
