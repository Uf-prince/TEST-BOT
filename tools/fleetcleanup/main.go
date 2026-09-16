// fleetcleanup: Storadera fleet-hash se stale fields delete karne ka one-off
// tool. Usage: ./fleetcleanup <field>   (jaise: ./fleetcleanup 172.19.217.219)
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	endpoint = "eu-east-1.s3.storadera.com"
	region   = "eu-east-1"
	access   = "AKIA6139I7AGJR0L0630"
	secret   = "ZPkK0Pkczn3kWetTMJGUyBjPUqvb9Z2TBXddIpsz"
	bucket   = "goldmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: fleetcleanup <field>")
		os.Exit(1)
	}
	field := os.Args[1]
	key := "kv/hashes/" + url.PathEscape("goldmd:fleet:servers") + "/" + url.PathEscape(field)
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: true,
		Region: region,
	})
	if err != nil {
		fmt.Println("ERR client:", err)
		os.Exit(1)
	}
	fmt.Println("DELETE s3://" + bucket + "/" + key)
	err = cli.RemoveObject(context.Background(), bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		fmt.Println("ERR delete:", err)
		os.Exit(1)
	}
	fmt.Println("DELETED OK:", field)
}
