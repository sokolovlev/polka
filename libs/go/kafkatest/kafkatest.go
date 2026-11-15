// Package kafkatest gives integration tests a real single-node Kafka (KRaft)
// from the apache/kafka image, the same as docker compose.
//
// Not the testcontainers kafka module: it targets confluentinc/confluent-local,
// which is under the Confluent Community License, not a free license (R6).
//
// The broker must advertise the host port Docker picks at start, which is
// unknown before the container starts. So the container waits for a start
// script that the test writes once the port is known (the same trick the
// testcontainers module uses).
package kafkatest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Image matches deploy/compose/compose.yaml.
const Image = "apache/kafka:4.4.0"

const startScript = "/tmp/start-kafka.sh"

var (
	once     sync.Once
	broker   string
	startErr error
)

func start() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, Image,
		testcontainers.WithExposedPorts("9092/tcp"),
		testcontainers.WithEntrypoint("sh"),
		testcontainers.WithCmd("-c", "while [ ! -f "+startScript+" ]; do sleep 0.1; done; . "+startScript),
		testcontainers.WithEnv(map[string]string{
			"KAFKA_NODE_ID":                                  "1",
			"KAFKA_PROCESS_ROLES":                            "broker,controller",
			"KAFKA_CONTROLLER_QUORUM_VOTERS":                 "1@localhost:9093",
			"KAFKA_LISTENERS":                                "PLAINTEXT://:9092,BROKER://:19092,CONTROLLER://:9093",
			"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "PLAINTEXT:PLAINTEXT,BROKER:PLAINTEXT,CONTROLLER:PLAINTEXT",
			"KAFKA_INTER_BROKER_LISTENER_NAME":               "BROKER",
			"KAFKA_CONTROLLER_LISTENER_NAMES":                "CONTROLLER",
			"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
			"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
			"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
			"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
			"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                "false",
			"CLUSTER_ID":                                     "polka-test-kraft-0001",
		}),
		// No wait strategy: Kafka only starts after the script below is
		// copied in; readiness is checked with a ping afterwards.
	)
	if err != nil {
		startErr = fmt.Errorf("start kafka container: %w", err)
		return
	}
	host, err := c.Host(ctx)
	if err != nil {
		startErr = err
		return
	}
	port, err := c.MappedPort(ctx, "9092/tcp")
	if err != nil {
		startErr = err
		return
	}
	broker = fmt.Sprintf("%s:%s", host, port.Port())
	script := fmt.Sprintf("export KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://%s,BROKER://localhost:19092\nexec /etc/kafka/docker/run\n", broker)
	if err := c.CopyToContainer(ctx, []byte(script), startScript, 0o755); err != nil {
		startErr = fmt.Errorf("copy start script: %w", err)
		return
	}
	startErr = waitReady(ctx)
}

func waitReady(ctx context.Context) error {
	cl, err := kgo.NewClient(kgo.SeedBrokers(broker))
	if err != nil {
		return fmt.Errorf("kafka client: %w", err)
	}
	defer cl.Close()
	for {
		if err := cl.Ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("kafka not ready: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Broker returns the bootstrap address of the shared test broker and
// creates the given topics (6 partitions, like compose). Skipped when Docker
// is not available.
func Broker(t *testing.T, topics ...string) string {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	once.Do(start)
	if startErr != nil {
		t.Fatal(startErr)
	}
	if len(topics) > 0 {
		cl, err := kgo.NewClient(kgo.SeedBrokers(broker))
		if err != nil {
			t.Fatal(err)
		}
		defer cl.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := kadm.NewClient(cl).CreateTopics(ctx, 6, 1, nil, topics...)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if r.Err != nil {
				t.Fatalf("create topic %s: %v", r.Topic, r.Err)
			}
		}
	}
	return broker
}
