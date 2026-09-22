package main

// ════════════════════════════════════════════════════════════════════════════
//   OBJECT-STORE ABSTRACTION  (owner order: Storj buckets TEMP comment karo,
//   unki jagah apna LOCAL DATABASE laga do)
//
//   Poora KV / message / JSON layer (storage.go) Storj ke UPAR baitha hai —
//   kv/strings, kv/sets, kv/hashes, msgs/, automsg/, <ns>/ object keys.
//   Sirf OBJECT STORE backend badalna hai to upar ka sab kuch (KV semantics,
//   message store, JSON store, disk cache) BILKUL waisa hi chalega.
//
//   Is liye ek `objStore` interface banaya gaya hai:
//     • minioStore → Storj/S3 (minio client)  [TEMP DISABLED]
//     • localStore → apna SQLite local DB      [ACTIVE]
//
//   Call sites (storage.go / diskcache.go / diskcache_helpers.go) sirf
//   `shard.client.<Method>(...)` call karte hain — interface ke through
//   dono backends transparently kaam karte hain.
// ════════════════════════════════════════════════════════════════════════════

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
)

// objReader is the minimal read+close surface every GetObject caller uses
// (io.ReadAll / bytes.Buffer.ReadFrom + Close). Both *minio.Object and the
// local in-memory reader satisfy it.
type objReader interface {
	io.Reader
	io.Closer
}

// objStore is the object-storage backend abstraction. Method signatures mirror
// the minio.Client surface EXACTLY (same option/return types) so call sites
// need zero changes beyond the `client` field type.
type objStore interface {
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucket, key string, opts minio.GetObjectOptions) (objReader, error)
	RemoveObject(ctx context.Context, bucket, key string, opts minio.RemoveObjectOptions) error
	ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
	StatObject(ctx context.Context, bucket, key string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	BucketExists(ctx context.Context, bucket string) (bool, error)
	MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error
}

// ── minioStore: Storj/S3 backend adapter (TEMP DISABLED — kept for switch-back) ──

type minioStore struct{ c *minio.Client }

func (m *minioStore) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return m.c.PutObject(ctx, bucket, key, r, size, opts)
}

func (m *minioStore) GetObject(ctx context.Context, bucket, key string, opts minio.GetObjectOptions) (objReader, error) {
	o, err := m.c.GetObject(ctx, bucket, key, opts)
	if err != nil {
		return nil, err
	}
	return o, nil
}

func (m *minioStore) RemoveObject(ctx context.Context, bucket, key string, opts minio.RemoveObjectOptions) error {
	return m.c.RemoveObject(ctx, bucket, key, opts)
}

func (m *minioStore) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	return m.c.ListObjects(ctx, bucket, opts)
}

func (m *minioStore) StatObject(ctx context.Context, bucket, key string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return m.c.StatObject(ctx, bucket, key, opts)
}

func (m *minioStore) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return m.c.BucketExists(ctx, bucket)
}

func (m *minioStore) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	return m.c.MakeBucket(ctx, bucket, opts)
}
