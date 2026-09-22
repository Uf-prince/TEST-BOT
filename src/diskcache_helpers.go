package main

// diskcache_helpers.go — minio helpers used by the disk-cache bulk loader.

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
)

// minioListOpts — recursive listing under a prefix (bulk-load helper).
func minioListOpts(prefix string) minio.ListObjectsOptions {
	return minio.ListObjectsOptions{Prefix: prefix, Recursive: true}
}

// kvReadShard — standalone object read (bulk-load helper; mirrors Upstash.kvRead).
func kvReadShard(ctx context.Context, sh *storjShard, key string) ([]byte, bool, error) {
	obj, err := sh.client.GetObject(ctx, sh.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if minioIsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		if minioIsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}
