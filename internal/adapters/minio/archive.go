package minioadapter

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"iot-platform/internal/model"
)

type Archive struct {
	client    *minio.Client
	rawBucket string
	rawQueue  chan rawRequest
	ensured   sync.Map
	sequence  atomic.Uint64
}

type rawRequest struct {
	ctx    context.Context
	value  model.RawMessage
	result chan rawResult
}
type rawResult struct {
	index model.RawArchiveIndex
	err   error
}

func New(endpoint, access, secret string, tls bool) (*Archive, error) {
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: tls})
	if err != nil {
		return nil, err
	}
	a := &Archive{client: c, rawBucket: "iot-raw-archive", rawQueue: make(chan rawRequest, 8192)}
	go a.batchLoop()
	return a, nil
}
func safe(v string) string { return strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(v) }
func (a *Archive) ensure(ctx context.Context, bucket string) error {
	if _, ok := a.ensured.Load(bucket); ok {
		return nil
	}
	ok, err := a.client.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !ok {
		if err = a.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	a.ensured.Store(bucket, struct{}{})
	return nil
}
func (a *Archive) PutRaw(ctx context.Context, m model.RawMessage) (model.RawArchiveIndex, error) {
	result := make(chan rawResult, 1)
	request := rawRequest{ctx: ctx, value: m, result: result}
	select {
	case a.rawQueue <- request:
	case <-ctx.Done():
		return model.RawArchiveIndex{}, ctx.Err()
	}
	select {
	case value := <-result:
		return value.index, value.err
	case <-ctx.Done():
		return model.RawArchiveIndex{}, ctx.Err()
	}
}

func (a *Archive) batchLoop() {
	for first := range a.rawQueue {
		batch := []rawRequest{first}
		timer := time.NewTimer(20 * time.Millisecond)
	collect:
		for len(batch) < 500 {
			select {
			case item := <-a.rawQueue:
				batch = append(batch, item)
			case <-timer.C:
				break collect
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		a.flushRawBatch(batch)
	}
}

func (a *Archive) flushRawBatch(batch []rawRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.ensure(ctx, a.rawBucket); err != nil {
		for _, item := range batch {
			item.result <- rawResult{err: err}
		}
		return
	}
	now := time.Now().UTC()
	key := fmt.Sprintf("batches/%04d/%02d/%02d/%02d/%02d/batch-%d-%d.jsonl.gz", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.UnixNano(), a.sequence.Add(1))
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	encoder := json.NewEncoder(gz)
	for _, item := range batch {
		if err := encoder.Encode(item.value); err != nil {
			_ = gz.Close()
			for _, pending := range batch {
				pending.result <- rawResult{err: err}
			}
			return
		}
	}
	if err := gz.Close(); err != nil {
		for _, item := range batch {
			item.result <- rawResult{err: err}
		}
		return
	}
	_, err := a.client.PutObject(ctx, a.rawBucket, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), minio.PutObjectOptions{ContentType: "application/gzip", UserMetadata: map[string]string{"batch-size": fmt.Sprint(len(batch))}})
	if err != nil {
		for _, item := range batch {
			item.result <- rawResult{err: err}
		}
		return
	}
	archivedAt := time.Now().UnixMilli()
	for offset, item := range batch {
		m := item.value
		item.result <- rawResult{index: model.RawArchiveIndex{MessageID: m.MessageID, TenantID: m.TenantID, ProductID: m.ProductID, DeviceID: m.DeviceID, Protocol: m.Protocol, PayloadFormat: m.PayloadFormat, ObjectBucket: a.rawBucket, ObjectKey: key, ObjectOffset: int64(offset), PayloadHash: m.PayloadHash(), PayloadSize: len(m.Payload), ReceivedAt: m.ReceivedAt, ArchivedAt: archivedAt}}
	}
}
func (a *Archive) GetRaw(ctx context.Context, idx model.RawArchiveIndex) (model.RawMessage, error) {
	var m model.RawMessage
	o, err := a.client.GetObject(ctx, idx.ObjectBucket, idx.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return m, err
	}
	defer o.Close()
	gz, err := gzip.NewReader(o)
	if err != nil {
		return m, err
	}
	defer gz.Close()
	decoder := json.NewDecoder(gz)
	for offset := int64(0); ; offset++ {
		if err = decoder.Decode(&m); err != nil {
			return model.RawMessage{}, err
		}
		if offset == idx.ObjectOffset || m.MessageID == idx.MessageID {
			return m, nil
		}
	}
}
func (a *Archive) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (string, error) {
	if err := a.ensure(ctx, bucket); err != nil {
		return "", err
	}
	if size < 0 {
		data, err := io.ReadAll(r)
		if err != nil {
			return "", err
		}
		r = bytes.NewReader(data)
		size = int64(len(data))
	}
	_, err := a.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return fmt.Sprintf("minio://%s/%s", bucket, key), err
}
func (a *Archive) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return a.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}
func (a *Archive) Health(ctx context.Context) error { _, err := a.client.ListBuckets(ctx); return err }
