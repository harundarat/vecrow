//go:build integration

package queue

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/requestid"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func openRabbit(t *testing.T, logs *syncBuffer) *RabbitMQ {
	t.Helper()
	url := os.Getenv("QUEUE_URL")
	if url == "" {
		t.Skip("QUEUE_URL not set")
	}
	log, err := logger.New(logger.Config{Level: "debug", Format: "json", Output: logs})
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenRabbitMQ(context.Background(), RabbitMQConfig{
		URL:            url,
		Exchange:       "vecrow.it",
		ConnectionName: "vecrow-integration-test",
		Prefetch:       2,
		MaxRetries:     2,
		RetryDelay:     200 * time.Millisecond,
		HandlerTimeout: 5 * time.Second,
		StartupTimeout: 5 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("OpenRabbitMQ: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// newTopic returns a unique topic whose queues are deleted after the test.
func newTopic(t *testing.T, r *RabbitMQ) string {
	t.Helper()
	topic := "it." + uuid.NewString()
	t.Cleanup(func() {
		ch, err := r.channel(context.Background())
		if err != nil {
			return
		}
		defer ch.Close()
		for _, q := range []string{topic, topic + retrySuffix, topic + deadSuffix} {
			_, _ = ch.QueueDelete(q, false, false, false)
		}
	})
	return topic
}

func subscribe(t *testing.T, r *RabbitMQ, topic string, h Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Subscribe(ctx, topic, h)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRabbitMQ_PublishConsumePropagatesRequestID(t *testing.T) {
	logs := &syncBuffer{}
	r := openRabbit(t, logs)
	topic := newTopic(t, r)

	type payload struct{ EscrowID string }
	got := make(chan Message, 1)
	subscribe(t, r, topic, func(ctx context.Context, msg Message) error {
		if requestid.From(ctx) != "req-it-1" {
			t.Errorf("handler ctx request id = %q", requestid.From(ctx))
		}
		got <- msg
		return nil
	})

	ctx := requestid.With(context.Background(), "req-it-1")
	if err := r.Publish(ctx, topic, payload{EscrowID: "esc-1"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg := <-got:
		var p payload
		if err := msg.Decode(&p); err != nil || p.EscrowID != "esc-1" || msg.Attempt != 1 || msg.ID == "" {
			t.Fatalf("message = %+v (%v)", msg, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("message not delivered")
	}
	waitFor(t, "processed log", func() bool { return strings.Contains(logs.String(), `"msg":"queue message processed"`) })
	if !strings.Contains(logs.String(), `"request_id":"req-it-1"`) {
		t.Errorf("consumer logs do not carry request_id:\n%s", logs)
	}
}

func TestRabbitMQ_RetriesThenSucceeds(t *testing.T) {
	r := openRabbit(t, &syncBuffer{})
	topic := newTopic(t, r)

	var attempts atomic.Int32
	var lastAttempt atomic.Int32
	subscribe(t, r, topic, func(_ context.Context, msg Message) error {
		lastAttempt.Store(int32(msg.Attempt))
		if attempts.Add(1) < 3 {
			return errors.New("transient failure")
		}
		return nil
	})
	if err := r.Publish(context.Background(), topic, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "third attempt", func() bool { return attempts.Load() >= 3 })
	if lastAttempt.Load() != 3 {
		t.Errorf("last attempt = %d, want 3", lastAttempt.Load())
	}
}

func TestRabbitMQ_DeadLetters(t *testing.T) {
	tests := map[string]Handler{
		"permanent error": func(context.Context, Message) error { return Permanent(errors.New("bad payload")) },
		// MaxRetries is 2: attempts 1, 2, 3 fail, then the message is dead-lettered.
		"retries exhausted": func(context.Context, Message) error { return errors.New("always fails") },
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			logs := &syncBuffer{}
			r := openRabbit(t, logs)
			topic := newTopic(t, r)
			subscribe(t, r, topic, h)

			if err := r.Publish(context.Background(), topic, map[string]string{"k": "v"}); err != nil {
				t.Fatal(err)
			}

			ch, err := r.channel(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer ch.Close()
			waitFor(t, "message in dead-letter queue", func() bool {
				q, err := ch.QueueDeclarePassive(topic+deadSuffix, true, false, false, false, nil)
				return err == nil && q.Messages == 1
			})
			// The message reaches the DLQ just before the log line is written.
			waitFor(t, "dead-letter log", func() bool {
				return strings.Contains(logs.String(), `"msg":"queue message dead-lettered"`)
			})
		})
	}
}

func TestRabbitMQ_HandlerTimeoutIsDeadLettered(t *testing.T) {
	r := openRabbit(t, &syncBuffer{})
	r.cfg.HandlerTimeout = 100 * time.Millisecond
	topic := newTopic(t, r)

	var attempts atomic.Int32
	subscribe(t, r, topic, func(ctx context.Context, _ Message) error {
		attempts.Add(1)
		<-ctx.Done()
		return ctx.Err()
	})
	if err := r.Publish(context.Background(), topic, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	ch, err := r.channel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	waitFor(t, "message in dead-letter queue", func() bool {
		q, err := ch.QueueDeclarePassive(topic+deadSuffix, true, false, false, false, nil)
		return err == nil && q.Messages == 1
	})
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}
