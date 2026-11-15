package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/sokolovlev/polka/libs/go/admin"
)

// KafkaPublisher publishes with franz-go (BSD-3, pure Go). Producer settings
// from spec 6.4: acks=all and idempotence (franz-go defaults, set explicitly
// here so they are visible). Idempotence keeps per-partition order across
// the producer's own retries.
type KafkaPublisher struct {
	client *kgo.Client
}

// NewKafkaPublisher creates a producer for brokers ("kafka:9092").
func NewKafkaPublisher(brokers []string, opts ...kgo.Opt) (*KafkaPublisher, error) {
	all := append([]kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(5 * time.Millisecond),
		kgo.RecordDeliveryTimeout(10 * time.Second),
	}, opts...)
	c, err := kgo.NewClient(all...)
	if err != nil {
		return nil, fmt.Errorf("kafka client: %w", err)
	}
	return &KafkaPublisher{client: c}, nil
}

// Publish implements Publisher. Records are produced concurrently (batched by
// the client) and the result is the length of the acknowledged prefix.
func (p *KafkaPublisher) Publish(ctx context.Context, msgs []Message) (int, error) {
	errs := make([]error, len(msgs))
	var wg sync.WaitGroup
	wg.Add(len(msgs))
	for i, m := range msgs {
		rec := &kgo.Record{Topic: m.Topic, Key: []byte(m.Key), Value: m.Payload}
		p.client.Produce(ctx, rec, func(_ *kgo.Record, err error) {
			errs[i] = err
			wg.Done()
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return i, fmt.Errorf("produce %s to %s: %w", msgs[i].EventID, msgs[i].Topic, err)
		}
	}
	return len(msgs), nil
}

// Check is the readiness check for Kafka.
func (p *KafkaPublisher) Check() admin.Check {
	return admin.Check{Name: "kafka", Check: func(ctx context.Context) error {
		if err := p.client.Ping(ctx); err != nil {
			return fmt.Errorf("ping: %w", err)
		}
		return nil
	}}
}

// Close flushes and closes the producer.
func (p *KafkaPublisher) Close() { p.client.Close() }
