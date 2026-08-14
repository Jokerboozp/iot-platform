package kafkaadapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"iot-platform/internal/ports"
)

type Bus struct {
	brokers []string
	mu      sync.Mutex
	writers map[string]*kafka.Writer
	readers []*kafka.Reader
	cancel  context.CancelFunc
}

func New(brokers []string) *Bus { return &Bus{brokers: brokers, writers: map[string]*kafka.Writer{}} }
func (b *Bus) writer(topic string) *kafka.Writer {
	b.mu.Lock()
	defer b.mu.Unlock()
	if w := b.writers[topic]; w != nil {
		return w
	}
	w := &kafka.Writer{Addr: kafka.TCP(b.brokers...), Topic: topic, Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false, AllowAutoTopicCreation: true, BatchSize: 500, BatchBytes: 4 << 20, BatchTimeout: 10 * time.Millisecond}
	b.writers[topic] = w
	return w
}
func (b *Bus) Publish(ctx context.Context, topic, key string, payload []byte) error {
	return b.writer(topic).WriteMessages(ctx, kafka.Message{Key: []byte(key), Value: payload})
}
func (b *Bus) Subscribe(ctx context.Context, topic, group string, h ports.Handler) error {
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: b.brokers, Topic: topic, GroupID: "iot-platform-" + group, MinBytes: 1, MaxBytes: 10e6, CommitInterval: 0})
	b.mu.Lock()
	b.readers = append(b.readers, reader)
	b.mu.Unlock()
	go func() {
		for {
			m, err := reader.FetchMessage(ctx)
			if err != nil {
				return
			}
			var handleErr error
			for attempt := 1; attempt <= 3; attempt++ {
				handleErr = h(ctx, m.Value)
				if handleErr == nil {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(attempt*attempt) * 250 * time.Millisecond):
				}
			}
			if handleErr != nil {
				dlq := append([]byte(fmt.Sprintf(`{"sourceTopic":%q,"consumerGroup":%q,"retryCount":3,"error":%q,"payload":`, topic, group, handleErr.Error())), append(m.Value, '}')...)
				if err := b.Publish(ctx, "iot.dlq."+group, string(m.Key), dlq); err != nil {
					continue
				}
				_ = reader.CommitMessages(ctx, m)
				continue
			}
			_ = reader.CommitMessages(ctx, m)
		}
	}()
	return nil
}
func (b *Bus) Health(ctx context.Context) error {
	if len(b.brokers) == 0 {
		return fmt.Errorf("no kafka brokers")
	}
	conn, err := kafka.DialContext(ctx, "tcp", b.brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Brokers()
	return err
}
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var errs []string
	for _, r := range b.readers {
		if err := r.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for _, w := range b.writers {
		if err := w.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
