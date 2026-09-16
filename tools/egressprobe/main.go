// Full project audit: ALL buckets, sizes, ages, oldest objects
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var shards = [10][3]string{
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

func newCli(sh [3]string) (*minio.Client, error) {
	return minio.New("gateway.storjshare.io:443", &minio.Options{
		Creds: credentials.NewStaticV4(sh[0], sh[1], ""), Region: "us-east-1", Secure: true,
	})
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	now := time.Now()

	// bucket inventory via shard-1 credentials (same project/account)
	cli, err := newCli(shards[0])
	if err != nil {
		fmt.Println("ERR client:", err)
		return
	}
	buckets, err := cli.ListBuckets(ctx)
	if err != nil {
		fmt.Println("LIST BUCKETS ERR:", err)
		return
	}
	fmt.Printf("=== ALL BUCKETS (%d) — creation dates (ACCOUNT AGE!) ===\n", len(buckets))
	for _, b := range buckets {
		ageDays := int(now.Sub(b.CreationDate).Hours() / 24)
		fmt.Printf("  %-12s created=%s  age=%dd\n", b.Name, b.CreationDate.Format("2006-01-02"), ageDays)
	}

	// per-bucket size (all buckets, kv+msgs+everything)
	fmt.Println("\n=== PER-BUCKET TOTAL USAGE ===")
	var total int64
	for _, b := range buckets {
		var sum int64
		var cnt int
		var oldest time.Time
		objCh := cli.ListObjects(ctx, b.Name, minio.ListObjectsOptions{Prefix: "", Recursive: true})
		for obj := range objCh {
			if obj.Err != nil {
				break
			}
			sum += obj.Size
			cnt++
			if oldest.IsZero() || obj.LastModified.Before(oldest) {
				oldest = obj.LastModified
			}
			if cnt >= 20000 {
				break
			}
		}
		total += sum
		fmt.Printf("  %-12s objs=%-6d size=%-10s oldest-obj=%s\n", b.Name, cnt, human(sum), oldest.Format("01-02 15:04"))
	}
	fmt.Printf("\nPROJECT TOTAL (bounded scan): %s\n", human(total))
}

func human(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
