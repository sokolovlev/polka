package outbox_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sokolovlev/polka/libs/go/kafkatest"
	"github.com/sokolovlev/polka/libs/go/outbox"
)

func TestKafkaPublisherDeliversInOrderPerKey(t *testing.T) {
	topic := fmt.Sprintf("outbox.test.%d", time.Now().UnixNano())
	broker := kafkatest.Broker(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pub, err := outbox.NewKafkaPublisher([]string{broker})
	require.NoError(t, err)
	defer pub.Close()
	require.NoError(t, pub.Check().Check(ctx))

	var msgs []outbox.Message
	for i := range 10 {
		msgs = append(msgs, outbox.Message{
			EventID: uuid.Must(uuid.NewV7()), Topic: topic,
			Key: "order-1", Payload: []byte(fmt.Sprintf("event-%d", i)),
		})
	}
	n, err := pub.Publish(ctx, msgs)
	require.NoError(t, err)
	assert.Equal(t, 10, n)

	consumer, err := kgo.NewClient(kgo.SeedBrokers(broker), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	defer consumer.Close()
	var got []string
	for len(got) < 10 {
		fetches := consumer.PollFetches(ctx)
		require.NoError(t, ctx.Err())
		fetches.EachRecord(func(r *kgo.Record) {
			assert.Equal(t, "order-1", string(r.Key))
			got = append(got, string(r.Value))
		})
	}
	for i, v := range got {
		assert.Equal(t, fmt.Sprintf("event-%d", i), v, "same key = same partition = same order")
	}
}

func TestKafkaPublisherReportsFailedPrefix(t *testing.T) {
	broker := kafkatest.Broker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pub, err := outbox.NewKafkaPublisher([]string{broker}, kgo.RecordDeliveryTimeout(3*time.Second))
	require.NoError(t, err)
	defer pub.Close()

	// Auto topic creation is off, like in compose: an unknown topic fails.
	n, err := pub.Publish(ctx, []outbox.Message{{
		EventID: uuid.Must(uuid.NewV7()), Topic: "no.such.topic", Key: "k", Payload: []byte("x"),
	}})
	require.Error(t, err)
	assert.Zero(t, n)
}
