package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"runtime/debug"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/internal/retry"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/requestid"
)

// Queue topology per topic is described in docs/architecture.md §7.3.
// Changing RetryDelay for an existing topic requires deleting T.retry first,
// because RabbitMQ rejects re-declaring a queue with different arguments.
const (
	defaultExchange = "vecrow.jobs"
	retrySuffix     = ".retry"
	deadSuffix      = ".dlq"

	headerAttempt   = "x-attempt"
	headerLastError = "x-last-error"
	headerRequestID = "x-request-id"

	dialTimeout       = 5 * time.Second
	heartbeat         = 10 * time.Second
	reconnectMinDelay = 500 * time.Millisecond
	reconnectMaxDelay = 30 * time.Second
	resubscribeDelay  = 2 * time.Second
	connPollInterval  = 200 * time.Millisecond
	maxErrorHeaderLen = 512
	settleTimeout     = 10 * time.Second
	requeueDelay      = time.Second
)

// RabbitMQConfig configures the RabbitMQ client.
type RabbitMQConfig struct {
	// URL in the form amqp://user:password@host:port/vhost.
	URL string
	// Exchange that topics are bound to. Defaults to "vecrow.jobs".
	Exchange string
	// ConnectionName is shown in the RabbitMQ management UI.
	ConnectionName string
	// Prefetch is the number of unacked messages, and concurrent handlers,
	// per subscription.
	Prefetch   int
	MaxRetries int
	RetryDelay time.Duration
	// HandlerTimeout bounds a single handler call. In-flight handlers keep
	// running (up to this timeout) after shutdown starts.
	HandlerTimeout time.Duration
	// StartupTimeout bounds how long OpenRabbitMQ waits for the broker.
	StartupTimeout time.Duration
}

// RabbitMQ implements Publisher and Subscriber. It reconnects automatically
// when the connection drops; subscriptions resume once it is back.
type RabbitMQ struct {
	cfg RabbitMQConfig
	log *slog.Logger

	mu     sync.RWMutex
	conn   *amqp.Connection
	closed bool
	done   chan struct{}

	// pubMu serialises publishing on a single confirm-mode channel.
	pubMu    sync.Mutex
	pubCh    *amqp.Channel
	declared map[string]bool
}

var (
	_ Publisher  = (*RabbitMQ)(nil)
	_ Subscriber = (*RabbitMQ)(nil)
)

// OpenRabbitMQ connects to the broker, retrying until StartupTimeout.
func OpenRabbitMQ(ctx context.Context, cfg RabbitMQConfig, log *slog.Logger) (*RabbitMQ, error) {
	if cfg.Exchange == "" {
		cfg.Exchange = defaultExchange
	}
	uri, err := amqp.ParseURI(cfg.URL)
	if err != nil {
		// The parse error may echo the URL, including its password.
		return nil, errors.New("queue: invalid QUEUE_URL (expected amqp://user:password@host:port/vhost)")
	}

	r := &RabbitMQ{
		cfg:  cfg,
		log:  log.With(slog.String("component", Name)),
		done: make(chan struct{}),
	}
	err = retry.Until(ctx, r.log, Name, cfg.StartupTimeout, func(context.Context) error {
		conn, err := r.dial()
		if err != nil {
			return err
		}
		r.conn = conn
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("queue: %w", err)
	}
	go r.watch(r.conn)

	r.log.InfoContext(ctx, "queue connected",
		slog.String("host", uri.Host),
		slog.Int("port", uri.Port),
		slog.String("vhost", uri.Vhost),
		slog.String("user", uri.Username),
		slog.String("exchange", cfg.Exchange),
	)
	return r, nil
}

func (r *RabbitMQ) dial() (*amqp.Connection, error) {
	props := amqp.NewConnectionProperties()
	props.SetClientConnectionName(r.cfg.ConnectionName)
	return amqp.DialConfig(r.cfg.URL, amqp.Config{
		Heartbeat:  heartbeat,
		Properties: props,
		Dial:       amqp.DefaultDial(dialTimeout),
	})
}

// watch waits for conn to drop and replaces it with a new connection.
func (r *RabbitMQ) watch(conn *amqp.Connection) {
	closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
	select {
	case <-r.done:
		return
	case amqpErr := <-closeCh:
		if r.isClosed() {
			return
		}
		attrs := []any{}
		if amqpErr != nil {
			attrs = append(attrs, logger.Err(amqpErr))
		}
		r.log.Warn("queue connection lost, reconnecting", attrs...)
	}

	delay := reconnectMinDelay
	for attempt := 1; ; attempt++ {
		select {
		case <-r.done:
			return
		case <-time.After(delay):
		}

		newConn, err := r.dial()
		if err != nil {
			r.log.Warn("queue reconnect failed",
				slog.Int("attempt", attempt),
				slog.Duration("retry_in", min(delay*2, reconnectMaxDelay)),
				logger.Err(err),
			)
			delay = min(delay*2, reconnectMaxDelay)
			continue
		}

		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			_ = newConn.Close()
			return
		}
		r.conn = newConn
		r.mu.Unlock()

		r.log.Info("queue reconnected", slog.Int("attempt", attempt))
		go r.watch(newConn)
		return
	}
}

func (r *RabbitMQ) isClosed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.closed
}

// channel opens a channel on the current connection, waiting while a
// reconnect is in progress.
func (r *RabbitMQ) channel(ctx context.Context) (*amqp.Channel, error) {
	for {
		r.mu.RLock()
		conn, closed := r.conn, r.closed
		r.mu.RUnlock()
		if closed {
			return nil, ErrClosed
		}
		if conn != nil && !conn.IsClosed() {
			ch, err := conn.Channel()
			if err == nil {
				return ch, nil
			}
			if !conn.IsClosed() {
				return nil, fmt.Errorf("queue: open channel: %w", err)
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.done:
			return nil, ErrClosed
		case <-time.After(connPollInterval):
		}
	}
}

func (r *RabbitMQ) declare(ch *amqp.Channel, topic string) error {
	if err := ch.ExchangeDeclare(r.cfg.Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare exchange %s: %w", r.cfg.Exchange, err)
	}
	if _, err := ch.QueueDeclare(topic, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare queue %s: %w", topic, err)
	}
	if err := ch.QueueBind(topic, topic, r.cfg.Exchange, false, nil); err != nil {
		return fmt.Errorf("queue: bind queue %s: %w", topic, err)
	}
	retryArgs := amqp.Table{
		"x-message-ttl":             r.cfg.RetryDelay.Milliseconds(),
		"x-dead-letter-exchange":    r.cfg.Exchange,
		"x-dead-letter-routing-key": topic,
	}
	if _, err := ch.QueueDeclare(topic+retrySuffix, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("queue: declare queue %s: %w", topic+retrySuffix, err)
	}
	if _, err := ch.QueueDeclare(topic+deadSuffix, true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare queue %s: %w", topic+deadSuffix, err)
	}
	return nil
}

// Publish returns once the broker has confirmed the message is stored.
func (r *RabbitMQ) Publish(ctx context.Context, topic string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: encode %s payload: %w", topic, err)
	}
	msg := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    uuid.NewString(),
		Timestamp:    time.Now().UTC(),
		Type:         topic,
		Headers:      amqp.Table{},
		Body:         body,
	}
	if id := requestid.From(ctx); id != "" {
		msg.Headers[headerRequestID] = id
	}

	// A failure is returned, not logged: the caller handles and logs it.
	if err := r.publish(ctx, topic, r.cfg.Exchange, topic, msg); err != nil {
		return fmt.Errorf("message %s: %w", msg.MessageId, err)
	}
	r.log.DebugContext(ctx, "queue message published",
		slog.String("topic", topic), slog.String("message_id", msg.MessageId))
	return nil
}

// publish sends msg with publisher confirms, reopening the channel once if it
// is broken.
func (r *RabbitMQ) publish(ctx context.Context, topic, exchange, key string, msg amqp.Publishing) error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	var lastErr error
	for range 2 {
		ch, err := r.publisherChannel(ctx, topic)
		if err != nil {
			return err
		}
		if lastErr = publishConfirmed(ctx, ch, exchange, key, msg); lastErr == nil {
			return nil
		}
		r.resetPublisher()
		if ctx.Err() != nil {
			break
		}
	}
	return fmt.Errorf("queue: publish to %s: %w", key, lastErr)
}

func publishConfirmed(ctx context.Context, ch *amqp.Channel, exchange, key string, msg amqp.Publishing) error {
	dc, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, msg)
	if err != nil {
		return err
	}
	ok, err := dc.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("broker did not confirm the message")
	}
	return nil
}

// publisherChannel returns the confirm-mode channel, opening it and declaring
// the topic's topology when needed. Callers hold pubMu.
func (r *RabbitMQ) publisherChannel(ctx context.Context, topic string) (*amqp.Channel, error) {
	if r.pubCh == nil || r.pubCh.IsClosed() {
		ch, err := r.channel(ctx)
		if err != nil {
			return nil, err
		}
		if err := ch.Confirm(false); err != nil {
			_ = ch.Close()
			return nil, fmt.Errorf("queue: enable publisher confirms: %w", err)
		}
		r.pubCh, r.declared = ch, map[string]bool{}
	}
	if !r.declared[topic] {
		if err := r.declare(r.pubCh, topic); err != nil {
			r.resetPublisher()
			return nil, err
		}
		r.declared[topic] = true
	}
	return r.pubCh, nil
}

// resetPublisher drops the publishing channel. Callers hold pubMu.
func (r *RabbitMQ) resetPublisher() {
	if r.pubCh != nil {
		_ = r.pubCh.Close()
	}
	r.pubCh, r.declared = nil, nil
}

// Subscribe blocks until ctx is cancelled, resubscribing after connection or
// channel failures.
func (r *RabbitMQ) Subscribe(ctx context.Context, topic string, h Handler) error {
	log := r.log.With(slog.String("topic", topic))
	for {
		err := r.consume(ctx, topic, h, log)
		if ctx.Err() != nil {
			log.InfoContext(ctx, "queue consumer stopped")
			return nil
		}
		if errors.Is(err, ErrClosed) || isPreconditionFailed(err) {
			// A topology conflict (for example a changed RetryDelay) never
			// resolves by retrying; fail the worker so it is noticed.
			return err
		}
		log.WarnContext(ctx, "queue consumer interrupted, resubscribing",
			slog.Duration("retry_in", resubscribeDelay), logger.Err(err))
		select {
		case <-ctx.Done():
			log.InfoContext(ctx, "queue consumer stopped")
			return nil
		case <-time.After(resubscribeDelay):
		}
	}
}

// isPreconditionFailed reports whether err is the broker rejecting a queue or
// exchange re-declaration with different arguments.
func isPreconditionFailed(err error) bool {
	var amqpErr *amqp.Error
	return errors.As(err, &amqpErr) && amqpErr.Code == amqp.PreconditionFailed
}

func (r *RabbitMQ) consume(ctx context.Context, topic string, h Handler, log *slog.Logger) error {
	ch, err := r.channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := r.declare(ch, topic); err != nil {
		return err
	}
	if err := ch.Qos(r.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("queue: set prefetch: %w", err)
	}
	deliveries, err := ch.ConsumeWithContext(ctx, topic, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("queue: consume %s: %w", topic, err)
	}
	log.InfoContext(ctx, "queue consumer started", slog.Int("prefetch", r.cfg.Prefetch))

	var wg sync.WaitGroup
	for range r.cfg.Prefetch {
		wg.Go(func() {
			for d := range deliveries {
				r.handle(ctx, topic, d, h)
			}
		})
	}
	wg.Wait()

	if ctx.Err() != nil {
		return nil
	}
	return errors.New("queue: delivery channel closed")
}

// handle runs h for one delivery and settles it: ack on success, otherwise
// forward to the retry or dead-letter queue and ack the original.
func (r *RabbitMQ) handle(parent context.Context, topic string, d amqp.Delivery, h Handler) {
	msg := Message{
		ID:          d.MessageId,
		Topic:       topic,
		Body:        d.Body,
		Headers:     stringHeaders(d.Headers),
		Attempt:     attemptOf(d.Headers),
		PublishedAt: d.Timestamp,
	}

	// Shutdown must not abort a handler half-way: detach from the consumer's
	// cancellation and bound the call with HandlerTimeout instead.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), r.cfg.HandlerTimeout)
	defer cancel()
	attrs := []slog.Attr{
		slog.String("topic", topic),
		slog.String("message_id", msg.ID),
		slog.Int("attempt", msg.Attempt),
	}
	if id := msg.Headers[headerRequestID]; requestid.Valid(id) {
		ctx = requestid.With(ctx, id)
		attrs = append(attrs, slog.String("request_id", id))
	}
	ctx = logger.WithAttrs(ctx, attrs...)

	r.log.DebugContext(ctx, "queue message received")
	start := time.Now()
	err := r.run(ctx, h, msg)
	elapsed := slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000)

	// The handler's deadline may have expired; settling needs its own.
	ctx, cancelSettle := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancelSettle()

	switch {
	case err == nil:
		r.ack(ctx, d)
		r.log.InfoContext(ctx, "queue message processed", elapsed)

	case IsPermanent(err) || msg.Attempt > r.cfg.MaxRetries:
		reason := "permanent error"
		if !IsPermanent(err) {
			reason = "retries exhausted"
		}
		if r.forward(ctx, d, topic, topic+deadSuffix, msg.Attempt, err) {
			r.ack(ctx, d)
			r.log.ErrorContext(ctx, "queue message dead-lettered",
				elapsed, slog.String("reason", reason), logger.Err(err))
		}

	default:
		if r.forward(ctx, d, topic, topic+retrySuffix, msg.Attempt+1, err) {
			r.ack(ctx, d)
			r.log.WarnContext(ctx, "queue message failed, retry scheduled",
				elapsed, slog.Duration("retry_in", r.cfg.RetryDelay), logger.Err(err))
		}
	}
}

// run calls h, converting a panic into an error so one bad message cannot
// crash the worker.
func (r *RabbitMQ) run(ctx context.Context, h Handler, msg Message) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.log.ErrorContext(ctx, "queue handler panicked",
				slog.String("panic", fmt.Sprint(p)), slog.String("stack", string(debug.Stack())))
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return h(ctx, msg)
}

// forward re-publishes d to queueName with the given attempt number and
// reports whether the original can be acked. If re-publishing fails the
// original is requeued so it is not lost.
func (r *RabbitMQ) forward(ctx context.Context, d amqp.Delivery, topic, queueName string, attempt int, cause error) bool {
	// Without a connection the delivery's channel is gone too: the broker
	// redelivers the unacked original after reconnecting. Return at once
	// instead of holding pubMu while waiting for the reconnect, which would
	// stall every other handler's forward behind this one.
	if err := r.Check(ctx); err != nil {
		r.log.WarnContext(ctx, "queue forward skipped, not connected; broker will redeliver",
			slog.String("target", queueName), logger.Err(err))
		return false
	}

	headers := amqp.Table{}
	maps.Copy(headers, d.Headers)
	headers[headerAttempt] = int32(attempt)
	headers[headerLastError] = truncate(cause.Error(), maxErrorHeaderLen)

	msg := amqp.Publishing{
		ContentType:  d.ContentType,
		DeliveryMode: amqp.Persistent,
		MessageId:    d.MessageId,
		Timestamp:    d.Timestamp,
		Type:         d.Type,
		Headers:      headers,
		Body:         d.Body,
	}
	if err := r.publish(ctx, topic, "", queueName, msg); err != nil {
		r.log.ErrorContext(ctx, "queue forward failed, requeueing message",
			slog.String("target", queueName), logger.Err(err))
		select {
		case <-ctx.Done():
		case <-time.After(requeueDelay):
		}
		if nackErr := d.Nack(false, true); nackErr != nil {
			r.log.ErrorContext(ctx, "queue nack failed", logger.Err(nackErr))
		}
		return false
	}
	return true
}

func (r *RabbitMQ) ack(ctx context.Context, d amqp.Delivery) {
	if err := d.Ack(false); err != nil {
		r.log.ErrorContext(ctx, "queue ack failed", logger.Err(err))
	}
}

func (r *RabbitMQ) Name() string { return Name }

func (r *RabbitMQ) Check(context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return ErrClosed
	}
	if r.conn == nil || r.conn.IsClosed() {
		return errors.New("queue: not connected")
	}
	return nil
}

func (r *RabbitMQ) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.done)
	conn := r.conn
	r.mu.Unlock()

	// Closing the connection first releases any publish waiting for a confirm.
	var err error
	if conn != nil && !conn.IsClosed() {
		err = conn.Close()
	}
	r.pubMu.Lock()
	r.resetPublisher()
	r.pubMu.Unlock()
	return err
}

func attemptOf(h amqp.Table) int {
	var n int
	switch v := h[headerAttempt].(type) {
	case int8:
		n = int(v)
	case int16:
		n = int(v)
	case int32:
		n = int(v)
	case int64:
		n = int(v)
	case int:
		n = v
	case string:
		n, _ = strconv.Atoi(v)
	}
	return max(n, 1)
}

func stringHeaders(h amqp.Table) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		switch v := v.(type) {
		case string:
			out[k] = v
		case []byte:
			out[k] = string(v)
		case int8, int16, int32, int64, int, uint8, uint16, uint32, uint64, bool, float32, float64:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary so the header stays valid UTF-8.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
