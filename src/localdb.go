package main

// ════════════════════════════════════════════════════════════════════════════
//   LOCAL DATABASE OBJECT STORE  (owner order — TEMP TEST)
//
//   Storj/Storadera buckets TEMP comment kar diye gaye hain. Unki jagah ye
//   apna LOCAL SQLite database (modernc.org/sqlite — pure-Go, CGO-free) hai.
//   Ye `objStore` interface implement karta hai, is liye storage.go ka poora
//   KV / message / JSON layer BINA kisi change ke isi pe chal jata hai.
//
//   Table: objects(bucket, key, body, ctype, umeta, ts)  PK(bucket,key)
//     • kv/strings/<key>          → string value
//     • kv/sets/<key>/<member>    → set member
//     • kv/hashes/<hash>/<field>  → hash field value
//     • msgs/<ts>/<id>.pb         → antidelete message proto
//     • automsg/<id>.json         → automsg config
//     • <ns>/<id>.json            → namespaced JSON (dissmisstime/admintime)
//
//   Sab kuch ek hi local file me — is liye saara session/config data aur
//   saare debugs ek jagah visible hain (owner requirement).
// ════════════════════════════════════════════════════════════════════════════

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
)

// localStore is the SQLite-backed objStore implementation.
type localStore struct {
	db   *sql.DB
	path string
	mu   sync.Mutex // serialises writes (SQLite single-writer safety)
}

var (
	localObjStore *localStore
	localStoreMu  sync.Mutex
)

// localDBDir — where the local object DB lives (env-overridable).
func localDBDir() string {
	if v := strings.TrimSpace(os.Getenv("GOLDMD_LOCALDB_DIR")); v != "" {
		return v
	}
	return filepath.Join(envOr("GOLDMD_DATA_DIR", "nexstore"), "localdb")
}

// InitLocalStore opens (creating if needed) the local SQLite object DB and
// returns the store. Idempotent — repeated calls return the same instance.
func InitLocalStore() (*localStore, error) {
	localStoreMu.Lock()
	defer localStoreMu.Unlock()
	if localObjStore != nil {
		return localObjStore, nil
	}
	dir := localDBDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("localdb: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "objstore.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("localdb: open: %w", err)
	}
	// modernc sqlite: keep a small pool; writes serialised by our mutex.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0)

	schema := `
CREATE TABLE IF NOT EXISTS objects (
  bucket TEXT NOT NULL,
  key    TEXT NOT NULL,
  body   BLOB,
  ctype  TEXT,
  umeta  TEXT,
  ts     INTEGER NOT NULL,
  PRIMARY KEY (bucket, key)
);
CREATE INDEX IF NOT EXISTS idx_objects_key ON objects(key);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("localdb: schema: %w", err)
	}
	ls := &localStore{db: db, path: path}
	localObjStore = ls
	JSONDebug("LOCALDB_INIT", map[string]any{"ok": true, "path": path})
	return ls, nil
}

// LocalStoreReady reports whether the local store is initialised.
func LocalStoreReady() bool {
	localStoreMu.Lock()
	defer localStoreMu.Unlock()
	return localObjStore != nil
}

// ── objStore implementation ─────────────────────────────────────────────────

func (l *localStore) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return minio.UploadInfo{}, fmt.Errorf("localdb put read: %w", err)
	}
	umeta := ""
	if len(opts.UserMetadata) > 0 {
		if b, e := json.Marshal(opts.UserMetadata); e == nil {
			umeta = string(b)
		}
	}
	now := time.Now().UnixMilli()
	l.mu.Lock()
	_, err = l.db.ExecContext(ctx,
		`INSERT INTO objects(bucket,key,body,ctype,umeta,ts) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(bucket,key) DO UPDATE SET body=excluded.body, ctype=excluded.ctype, umeta=excluded.umeta, ts=excluded.ts`,
		bucket, key, data, opts.ContentType, umeta, now)
	l.mu.Unlock()
	if err != nil {
		JSONDebug("LOCALDB_PUT", map[string]any{"ok": false, "bucket": bucket, "key": kvLogKey(key), "error": err.Error()})
		return minio.UploadInfo{}, fmt.Errorf("localdb put: %w", err)
	}
	JSONDebug("LOCALDB_PUT", map[string]any{"ok": true, "bucket": bucket, "key": kvLogKey(key), "bytes": len(data)})
	return minio.UploadInfo{Bucket: bucket, Key: key, Size: int64(len(data)), LastModified: time.Now()}, nil
}

func (l *localStore) GetObject(ctx context.Context, bucket, key string, opts minio.GetObjectOptions) (objReader, error) {
	var body []byte
	err := l.db.QueryRowContext(ctx, `SELECT body FROM objects WHERE bucket=? AND key=?`, bucket, key).Scan(&body)
	if err == sql.ErrNoRows {
		return nil, errNoSuchKey(key)
	}
	if err != nil {
		return nil, fmt.Errorf("localdb get: %w", err)
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (l *localStore) RemoveObject(ctx context.Context, bucket, key string, opts minio.RemoveObjectOptions) error {
	l.mu.Lock()
	_, err := l.db.ExecContext(ctx, `DELETE FROM objects WHERE bucket=? AND key=?`, bucket, key)
	l.mu.Unlock()
	if err != nil {
		return fmt.Errorf("localdb remove: %w", err)
	}
	return nil
}

func (l *localStore) ListObjects(ctx context.Context, bucket string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	go func() {
		defer close(ch)
		prefix := opts.Prefix
		rows, err := l.db.QueryContext(ctx,
			`SELECT key, COALESCE(LENGTH(body),0), ts FROM objects WHERE bucket=? AND key LIKE ? ESCAPE '\' ORDER BY key`,
			bucket, likePrefix(prefix))
		if err != nil {
			select {
			case ch <- minio.ObjectInfo{Err: err}:
			case <-ctx.Done():
			}
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var size int64
			var ts int64
			if err := rows.Scan(&key, &size, &ts); err != nil {
				continue
			}
			oi := minio.ObjectInfo{
				Key:          key,
				Size:         size,
				LastModified: time.UnixMilli(ts),
			}
			select {
			case ch <- oi:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (l *localStore) StatObject(ctx context.Context, bucket, key string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	var size int64
	var ts int64
	err := l.db.QueryRowContext(ctx, `SELECT COALESCE(LENGTH(body),0), ts FROM objects WHERE bucket=? AND key=?`, bucket, key).Scan(&size, &ts)
	if err == sql.ErrNoRows {
		return minio.ObjectInfo{}, errNoSuchKey(key)
	}
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("localdb stat: %w", err)
	}
	return minio.ObjectInfo{Key: key, Size: size, LastModified: time.UnixMilli(ts)}, nil
}

func (l *localStore) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return true, nil // local DB me buckets implicit hain
}

func (l *localStore) MakeBucket(ctx context.Context, bucket string, opts minio.MakeBucketOptions) error {
	return nil // no-op
}

// ── helpers ─────────────────────────────────────────────────────────────────

// errNoSuchKey returns an error whose text contains "NoSuchKey" so the
// existing minioIsNotFound() helper recognises it as a missing object.
func errNoSuchKey(key string) error {
	return errors.New("NoSuchKey: The specified key does not exist: " + key)
}

// likePrefix converts a plain prefix into a SQL LIKE pattern with escaping.
func likePrefix(prefix string) string {
	if prefix == "" {
		return "%"
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)
	return esc + "%"
}

// LocalStoreStats returns a small summary for the panel / debug (row count,
// distinct buckets, file size) so the owner can see the local DB is live.
func LocalStoreStats() map[string]any {
	localStoreMu.Lock()
	ls := localObjStore
	localStoreMu.Unlock()
	out := map[string]any{"ready": ls != nil}
	if ls == nil {
		return out
	}
	var rows int64
	_ = ls.db.QueryRow(`SELECT COUNT(*) FROM objects`).Scan(&rows)
	out["rows"] = rows
	out["path"] = ls.path
	if fi, err := os.Stat(ls.path); err == nil {
		out["bytes"] = fi.Size()
	}
	return out
}
