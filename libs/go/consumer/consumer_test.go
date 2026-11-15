package consumer_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sokolovlev/polka/libs/go/consumer"
	"github.com/sokolovlev/polka/libs/go/kafkatest"
	"github.com/sokolovlev/polka/libs/go/outbox"
	commonv1 "github.com/sokolovlev/polka/libs/go/proto/polka/common/v1"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// setup creates a unique topic and its DLQ and returns the broker and topic.
func setup(t *testing.T) (string, string) {
	t.Helper()
	topic := fmt.Sprintf("consumer.test.%d", time.Now().UnixNano())
	return kafkatest.Broker(t, topic, topic+".dlq"), topic
}

func produce(ctx context.Context, t *testing.T, broker, topic string, values ...[]byte) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(broker))
	require.NoError(t, err)
	defer cl.Close()
	for _, v := range values {
		require.NoError(t, cl.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte("agg-1"), Value: v}).FirstErr())
	}
}

func envelope(t *testing.T, n int64) []byte {
	t.Helper()
	m, err := outbox.NewMessage(outbox.Event{
		Topic: "x", Type: "test.happened", AggregateID: "agg-1",
		Payload: &commonv1.Money{AmountMinor: n, Currency: "RUB"},
	}, time.Now())
	require.NoError(t, err)
	return m.Payload
}

func start(t *testing.T, cfg consumer.Config) *consumer.Consumer {
	t.Helper()
	cfg.Log = quiet
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = time.Millisecond
	}
	c, err := consumer.New(cfg)
	require.NoError(t, err)
	go func() { _ = c.Run(context.Background()) }()
	return c
}

func stop(t *testing.T, c *consumer.Consumer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Shutdown(ctx))
}

func dlqRecords(ctx context.Context, t *testing.T, broker, dlq string, want int) []*kgo.Record {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(broker), kgo.ConsumeTopics(dlq),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	defer cl.Close()
	var out []*kgo.Record
	for len(out) < want {
		fetches := cl.PollFetches(ctx)
		require.NoError(t, ctx.Err(), "waiting for %d DLQ records, got %d", want, len(out))
		out = append(out, fetches.Records()...)
	}
	return out
}

func header(r *kgo.Record, key string) string {
	for _, h := range r.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func TestHandlesInOrderAndCommits(t *testing.T) {
	broker, topic := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	produce(ctx, t, broker, topic, envelope(t, 1), envelope(t, 2), envelope(t, 3))

	var mu sync.Mutex
	var seen []int64
	handler := func(_ context.Context, env *commonv1.Envelope, _ *kgo.Record) error {
		var m commonv1.Money
		if err := env.GetPayload().UnmarshalTo(&m); err != nil {
			return err
		}
		mu.Lock()
		seen = append(seen, m.GetAmountMinor())
		mu.Unlock()
		return nil
	}
	cfg := consumer.Config{Brokers: []string{broker}, Group: "g-" + topic, Topics: []string{topic}, Handler: handler}
	c := start(t, cfg)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) == 3 }, 30*time.Second, 50*time.Millisecond)
	stop(t, c)
	assert.Equal(t, []int64{1, 2, 3}, seen)

	// Same group again: offsets were committed, nothing is redelivered.
	produce(ctx, t, broker, topic, envelope(t, 4))
	c = start(t, cfg)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(seen) == 4 }, 30*time.Second, 50*time.Millisecond)
	stop(t, c)
	assert.Equal(t, []int64{1, 2, 3, 4}, seen)
}

func TestRetriesThenSucceeds(t *testing.T) {
	broker, topic := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	produce(ctx, t, broker, topic, envelope(t, 1))

	var calls atomic.Int32
	handler := func(context.Context, *commonv1.Envelope, *kgo.Record) error {
		if calls.Add(1) < 3 {
			return errors.New("database is restarting")
		}
		return nil
	}
	c := start(t, consumer.Config{Brokers: []string{broker}, Group: "g-" + topic, Topics: []string{topic}, Handler: handler})
	require.Eventually(t, func() bool { return calls.Load() == 3 }, 30*time.Second, 20*time.Millisecond)
	stop(t, c)
	assert.EqualValues(t, 3, calls.Load(), "two failures, then success; no further calls")
}

func TestFailingMessageGoesToDLQAndNextIsHandled(t *testing.T) {
	broker, topic := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bad, good := envelope(t, 1), envelope(t, 2)
	produce(ctx, t, broker, topic, bad, good)

	var calls atomic.Int32
	var goodHandled atomic.Bool
	handler := func(_ context.Context, env *commonv1.Envelope, _ *kgo.Record) error {
		var m commonv1.Money
		_ = env.GetPayload().UnmarshalTo(&m)
		if m.GetAmountMinor() == 1 {
			calls.Add(1)
			return errors.New("invalid state")
		}
		goodHandled.Store(true)
		return nil
	}
	c := start(t, consumer.Config{Brokers: []string{broker}, Group: "g-" + topic, Topics: []string{topic}, Handler: handler})
	require.Eventually(t, goodHandled.Load, 30*time.Second, 20*time.Millisecond)
	stop(t, c)

	assert.EqualValues(t, 6, calls.Load(), "1 attempt + 5 retries (spec 5.3)")
	recs := dlqRecords(ctx, t, broker, topic+".dlq", 1)
	assert.Equal(t, bad, recs[0].Value, "the original message is kept")
	assert.Equal(t, "agg-1", string(recs[0].Key))
	assert.Equal(t, "invalid state", header(recs[0], consumer.HeaderError))
	assert.Equal(t, topic, header(recs[0], consumer.HeaderTopic))
	assert.Equal(t, "6", header(recs[0], consumer.HeaderAttempts))
}

func TestPoisonMessageGoesToDLQWithoutRetries(t *testing.T) {
	broker, topic := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	produce(ctx, t, broker, topic, []byte("not a protobuf envelope \xff\xff"))

	var calls atomic.Int32
	handler := func(context.Context, *commonv1.Envelope, *kgo.Record) error { calls.Add(1); return nil }
	c := start(t, consumer.Config{Brokers: []string{broker}, Group: "g-" + topic, Topics: []string{topic}, Handler: handler})
	recs := dlqRecords(ctx, t, broker, topic+".dlq", 1)
	stop(t, c)

	assert.Zero(t, calls.Load(), "a message that cannot be decoded never reaches the handler")
	assert.Contains(t, header(recs[0], consumer.HeaderError), "decode envelope")
	assert.Equal(t, "0", header(recs[0], consumer.HeaderAttempts))
}

func TestPanicIsRetriedLikeAnError(t *testing.T) {
	broker, topic := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	produce(ctx, t, broker, topic, envelope(t, 1))

	var calls atomic.Int32
	handler := func(context.Context, *commonv1.Envelope, *kgo.Record) error {
		if calls.Add(1) == 1 {
			panic("nil map")
		}
		return nil
	}
	c := start(t, consumer.Config{Brokers: []string{broker}, Group: "g-" + topic, Topics: []string{topic}, Handler: handler})
	require.Eventually(t, func() bool { return calls.Load() == 2 }, 30*time.Second, 20*time.Millisecond)
	stop(t, c)
}
