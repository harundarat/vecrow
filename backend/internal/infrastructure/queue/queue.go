// Package queue provides durable background job messaging: publishers enqueue
// JSON messages on a topic and the worker consumes them with retries.
//
// Callers depend on Message, Handler, Publisher and Subscriber only; the
// RabbitMQ implementation is in rabbitmq.go.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Name is the component name used in logs and readiness checks.
const Name = "queue"

// ErrClosed is returned by operations on a closed queue client.
var ErrClosed = errors.New("queue: client closed")

// Message is one delivery of a published payload.
type Message struct {
	ID          string
	Topic       string
	Body        []byte
	Headers     map[string]string
	Attempt     int // 1 on the first delivery
	PublishedAt time.Time
}

// Decode unmarshals the JSON body into v. A malformed body can never succeed,
// so the error is permanent and the message goes straight to the dead-letter
// queue.
func (m Message) Decode(v any) error {
	if err := json.Unmarshal(m.Body, v); err != nil {
		return Permanent(fmt.Errorf("queue: decode %s message: %w", m.Topic, err))
	}
	return nil
}

// Handler processes one message. Returning nil acknowledges it. Returning an
// error schedules a retry, unless the error is wrapped with Permanent or the
// retry budget is exhausted, in which case the message is dead-lettered.
// Handlers must be idempotent: delivery is at least once.
type Handler func(ctx context.Context, msg Message) error

// Publisher enqueues payload (encoded as JSON) on topic.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload any) error
}

// Subscriber consumes topic with h until ctx is cancelled.
type Subscriber interface {
	Subscribe(ctx context.Context, topic string, h Handler) error
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as not retryable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}
