// storjpurge: PURANA Storj cleanup — saare umar/umar2..10 buckets ka SAMSA
// CONTENT delete karke buckets bhi hata deta hai (owner: jani, "storj nikal
// de sare buckets khali kr de"). Ye jaan-boojh kar final action hai — old
// data (1.4GB kv/ junk + archived messages) intentionally dropped.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	endpoint = "gateway.storjshare.io:443"
	region   = "us-east-1"
)

// Purane hardcoded Storj creds (storage.go ka OLD mirror — sirf purge ke liye).
var oldShards = [10][3]string{
	{"jwfjua62w45i5ebrx3t5nex455ma", "j2d2e4gjkc43m5sosfe7bznevm25aq627hgljdfhjofr5ezrsxazk", "umar"},
	{"jwpiqh2d4ky56hrjrrhrpz7h47cq", "j2n53jczyngsdci5vpr47ni24mlkdhzzrh2deyq2h4qelsst62da6", "umar2"},
	{"jvkcczaogv7syhlhyss2oot3dliq", "j327kiloo7yhd4x6n6epcvfbzfjuekjrf6bc237cesrv3nohh5kkc", "umar3"},
	{"jwknkytkfzph7mtonxq6g6d4ze3q", "jz24qglsu5wl34kmbsgchpgt2fzyibbdwyyehvqbe6pxvv4pzkpb2", "umar4"},
	{"jwjsgr627fnccmxfc4gtllg5bb7q", "jzihzet2ecmyn3inuglzvxldx3d5i6jnsrs4ky35nsr5tenxro7hg", "umar5"},
	{"jxzvkrhsaebljlko6dv2lin7x4wa", "j3iyzcwnbirmrhup3hc6352gl5n56xfhkna2l4dn3ugm4i7a2gd6s", "umar6"},
	{"jw6pkivs3vp6rmdty2da36auzimq", "j33i2ybq7kd7ouw6w7ltfmew2dzeg2t3v4ryterop75kdeyjdvvvo", "umar7"},
	{"ju4a4oqbejb3w7ygbmikkr4vgsna", "jzo2xqmutggpkbwxpgw5fswerf35miykdavdcfimmghqdpkgyqpxe", "umar8"},
	{"juznozmcpfsbwoqboijqwpus3raa", "j236o3cx4eud3dxraq55lnsnod4aradltsekqsbwk2cv6iebbtwma", "umar9"},
	{"ju7o5eflwumsaxxhdgdy6y23nbsq", "j3oulw7wfaequvvm5ims7xzjdkr5kfqgwecogozv4v25r72ffysog", "umar10"},
}

func main() {
	fmt.Println("STORJ PURGE START")
	t0 := time.Now()
	ctx := context.Background()
	totalDeleted := 0
	for i, sh := range oldShards {
		if sh[0] == "" || sh[1] == "" || sh[2] == "" {
			continue
		}
		fmt.Printf("[%d] bucket %s ...\n", i+1, sh[2])
		cli, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(sh[0], sh[1], ""),
			Secure: true,
			Region: region,
		})
		if err != nil {
			fmt.Println("  ERR client:", err)
			continue
		}
		bucket := sh[2]
		exists, err := cli.BucketExists(ctx, bucket)
		if err != nil {
			fmt.Println("  ERR bucket-exists:", err)
			continue
		}
		if !exists {
			fmt.Println("  (bucket already gone)")
			continue
		}
		// 1) saare objects delete
		objCh := cli.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, MaxKeys: 1000})
		n := 0
		for obj := range objCh {
			if obj.Err != nil {
				fmt.Println("  list err:", obj.Err)
				break
			}
			if err := cli.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
				fmt.Println("  del err:", obj.Key, err)
			} else {
				n++
				if n%500 == 0 {
					fmt.Printf("  ... %d objects deleted\n", n)
				}
			}
		}
		// 2) khali bucket remove
		if err := cli.RemoveBucket(ctx, bucket); err != nil {
			fmt.Printf("  deleted %d objects, bucket-remove err: %v\n", n, err)
		} else {
			fmt.Printf("  deleted %d objects, bucket REMOVED\n", n)
		}
		totalDeleted += n
	}
	fmt.Printf("STORJ PURGE DONE — %d objects, %.1fs\n", totalDeleted, time.Since(t0).Seconds())
}
