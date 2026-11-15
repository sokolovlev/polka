// Package consumer runs a Kafka consumer group with the delivery rules of
// spec 5.3 and 6.4:
//
//   - each message is an Envelope; one that cannot be decoded is poison and
//     goes straight to the DLQ;
//   - a failing handler is retried with exponential backoff (100 ms, 200 ms,
//     ... 1.6 s: 1 attempt + 5 retries), then the message goes to
//     <topic>.dlq with the error in headers;
//   - offsets are committed only after a message is handled or dead-lettered
//     (at-least-once). Handlers make it effectively once with package inbox.
//
// Records of one poll are handled sequentially, so per-partition order is
// kept. Rebalances are blocked while a poll is being processed, so a
// partition is never handed to another replica mid-batch.
package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/sokolovlev/polka/libs/go/admin"
	commonv1 "github.com/sokolovlev/polka/libs/go/proto/polka/common/v1"
)

// Handler processes one message. Returning an error triggers a retry.
type Handler func(ctx context.Context, env *commonv1.Envelope, rec *kgo.Record) error

// Config of a consumer group.
type Config struct {
	Brokers []string
	Group   string
	Topics  []string
	Handler Handler
	Log     *slog.Logger
	// Retries after the first attempt (spec: 5).
	Retries int
	// BaseBackoff doubles on every retry (spec: 100 ms → 1.6 s).
	BaseBackoff time.Duration
	// HandlerTimeout bounds one attempt.
	HandlerTimeout time.Duration
}

// DLQ header names.
const (
	HeaderError     = "x-error"
	HeaderTopic     = "x-original-topic"
	HeaderPartition = "x-original-partition"
	HeaderOffset    = "x-original-offset"
	HeaderAttempts  = "x-attempts"
)

// Consumer is a lifecycle.Component.
type Consumer struct {
	cfg    Config
	client *kgo.Client

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// New joins the group lazily (on the first poll).
func New(cfg Config) (*Consumer, error) {
	if cfg.Retries == 0 {
		cfg.Retries = 5
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}
	if cfg.HandlerTimeout == 0 {
		cfg.HandlerTimeout = 10 * time.Second
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		// The DLQ producer shares the client (spec 6.4: acks=all, idempotent).
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	return &Consumer{cfg: cfg, client: client, stop: make(chan struct{}), done: make(chan struct{})}, nil
}

// Name implements lifecycle.Component.
func (c *Consumer) Name() string { return "consumer:" + c.cfg.Group }

// Check is the readiness check for Kafka.
func (c *Consumer) Check() admin.Check {
	return admin.Check{Name: "kafka", Check: func(ctx context.Context) error {
		if err := c.client.Ping(ctx); err != nil {
			return fmt.Errorf("ping: %w", err)
		}
		return nil
	}}
}

// Run polls until Shutdown.
func (c *Consumer) Run(ctx context.Context) error {
	defer close(c.done)
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.stop:
			cancel() // unblocks PollFetches; the current batch is already done
		case <-pollCtx.Done():
		}
	}()

	for {
		fetches := c.client.PollFetches(pollCtx)
		if pollCtx.Err() != nil {
			c.client.AllowRebalance()
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			c.cfg.Log.Warn("kafka fetch error", slog.String("topic", topic),
				slog.Int("partition", int(partition)), slog.Any("error", err))
		})
		var handled []*kgo.Record
		for _, rec := range fetches.Records() {
			// Not interrupted by Shutdown: a started message is finished,
			// unless even the DLQ is unreachable (then it is not committed).
			if !c.process(context.WithoutCancel(ctx), rec) {
				break
			}
			handled = append(handled, rec)
		}
		if len(handled) > 0 {
			if err := c.client.CommitRecords(context.WithoutCancel(ctx), handled...); err != nil {
				// Not fatal: the records are redelivered and deduplicated by the inbox.
				c.cfg.Log.Warn("kafka commit failed", slog.Any("error", err))
			}
		}
		c.client.AllowRebalance()
	}
}

// process handles one record: retries, then DLQ. It returns true when the
// record is done (handled or dead-lettered) and its offset may be committed.
func (c *Consumer) process(ctx context.Context, rec *kgo.Record) bool {
	var env commonv1.Envelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return c.deadLetter(ctx, rec, fmt.Errorf("decode envelope: %w", err), 0)
	}
	var err error
	attempts := 0
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(c.cfg.BaseBackoff << (attempt - 1))
		}
		attempts++
		err = c.attempt(ctx, &env, rec)
		if err == nil {
			return true
		}
		c.cfg.Log.Warn("handler failed", slog.String("topic", rec.Topic),
			slog.String("event_id", env.GetEventId()), slog.Int("attempt", attempts), slog.Any("error", err))
	}
	return c.deadLetter(ctx, rec, err, attempts)
}

func (c *Consumer) attempt(ctx context.Context, env *commonv1.Envelope, rec *kgo.Record) (err error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.HandlerTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return c.cfg.Handler(ctx, env, rec)
}

// deadLetter copies rec to <topic>.dlq with the error in headers. If that
// fails it keeps trying until Shutdown, and then returns false: the offset is
// not committed and the message is redelivered after restart.
func (c *Consumer) deadLetter(ctx context.Context, rec *kgo.Record, cause error, attempts int) bool {
	dlq := &kgo.Record{
		Topic: rec.Topic + ".dlq",
		Key:   rec.Key,
		Value: rec.Value,
		Headers: append(append([]kgo.RecordHeader{}, rec.Headers...),
			kgo.RecordHeader{Key: HeaderError, Value: []byte(cause.Error())},
			kgo.RecordHeader{Key: HeaderTopic, Value: []byte(rec.Topic)},
			kgo.RecordHeader{Key: HeaderPartition, Value: []byte(strconv.Itoa(int(rec.Partition)))},
			kgo.RecordHeader{Key: HeaderOffset, Value: []byte(strconv.FormatInt(rec.Offset, 10))},
			kgo.RecordHeader{Key: HeaderAttempts, Value: []byte(strconv.Itoa(attempts))},
		),
	}
	for backoff := c.cfg.BaseBackoff; ; backoff = min(backoff*2, 5*time.Second) {
		err := c.client.ProduceSync(ctx, dlq).FirstErr()
		if err == nil {
			c.cfg.Log.Error("message sent to DLQ", slog.String("dlq", dlq.Topic),
				slog.Int64("offset", rec.Offset), slog.Any("error", cause))
			return true
		}
		c.cfg.Log.Error("DLQ produce failed, retrying", slog.String("dlq", dlq.Topic), slog.Any("error", err))
		select {
		case <-c.stop:
			return false
		case <-time.After(backoff):
		}
	}
}

// Shutdown stops polling after the current batch, commits its offsets and
// leaves the group, so partitions move to other replicas at once.
func (c *Consumer) Shutdown(ctx context.Context) error {
	c.stopOnce.Do(func() { close(c.stop) })
	select {
	case <-c.done:
	case <-ctx.Done():
		c.client.Close()
		return fmt.Errorf("consumer %s: %w", c.cfg.Group, ctx.Err())
	}
	c.client.Close()
	return nil
}
