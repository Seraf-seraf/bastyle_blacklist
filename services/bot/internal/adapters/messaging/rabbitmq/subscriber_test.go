package rabbitmq

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestIndexSubscriberConfigBuildsReplicaQueueName(t *testing.T) {
	cfg := IndexSubscriberConfig{
		URL:             "amqp://guest:guest@localhost:5672/",
		Exchange:        "bastyle.events",
		ExchangeType:    "topic",
		ReplicaID:       "replica-1",
		QueueTemplate:   "bastyle.replica.%s.events",
		RoutingKeys:     []string{"media.ban.#", "index.#"},
		Prefetch:        10,
		ReconnectDelay:  time.Second,
		CatchUpInterval: time.Second,
	}

	queue, err := cfg.QueueName()
	if err != nil {
		t.Fatal(err)
	}
	if queue != "bastyle.replica.replica-1.events" {
		t.Fatalf("неожиданное имя очереди: %q", queue)
	}
}

func TestIndexSubscriberUsesWatermillReconnectConfig(t *testing.T) {
	subscriber, err := NewIndexSubscriber(IndexSubscriberConfig{
		URL:             "amqp://guest:guest@localhost:5672/",
		Exchange:        "bastyle.events",
		ExchangeType:    "topic",
		ReplicaID:       "replica-1",
		QueueTemplate:   "bastyle.replica.%s.events",
		RoutingKeys:     []string{"media.ban.#"},
		Prefetch:        1,
		ReconnectDelay:  2 * time.Second,
		CatchUpInterval: time.Second,
	}, &fakeSynchronizer{})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := subscriber.watermillConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connection.Reconnect == nil {
		t.Fatal("ожидался Watermill reconnect config")
	}
	if cfg.Connection.Reconnect.BackoffInitialInterval != 2*time.Second {
		t.Fatalf("неожиданный reconnect interval: %s", cfg.Connection.Reconnect.BackoffInitialInterval)
	}
	if cfg.Consume.Qos.PrefetchCount != 1 {
		t.Fatalf("неожиданный prefetch: %d", cfg.Consume.Qos.PrefetchCount)
	}
}

type fakeSynchronizer struct{}

func (fakeSynchronizer) CatchUpAllIndexes(_ context.Context) error {
	return nil
}

func TestIndexSubscriberConfigUsesHostnameWhenReplicaIDEmpty(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	cfg := IndexSubscriberConfig{
		URL:             "amqp://guest:guest@localhost:5672/",
		Exchange:        "bastyle.events",
		ExchangeType:    "topic",
		QueueTemplate:   "bastyle.replica.%s.events",
		RoutingKeys:     []string{"media.ban.#", "index.#"},
		Prefetch:        10,
		ReconnectDelay:  time.Second,
		CatchUpInterval: time.Second,
	}

	queue, err := cfg.QueueName()
	if err != nil {
		t.Fatal(err)
	}
	if queue != "bastyle.replica."+host+".events" {
		t.Fatalf("неожиданное имя очереди: %q", queue)
	}
}

func TestIndexSubscriberConfigRejectsInvalidValues(t *testing.T) {
	tests := map[string]IndexSubscriberConfig{
		"url": {
			Exchange:        "bastyle.events",
			ExchangeType:    "topic",
			QueueTemplate:   "bastyle.replica.%s.events",
			RoutingKeys:     []string{"media.ban.#"},
			Prefetch:        10,
			ReconnectDelay:  time.Second,
			CatchUpInterval: time.Second,
		},
		"routing_keys": {
			URL:             "amqp://guest:guest@localhost:5672/",
			Exchange:        "bastyle.events",
			ExchangeType:    "topic",
			QueueTemplate:   "bastyle.replica.%s.events",
			Prefetch:        10,
			ReconnectDelay:  time.Second,
			CatchUpInterval: time.Second,
		},
		"prefetch": {
			URL:             "amqp://guest:guest@localhost:5672/",
			Exchange:        "bastyle.events",
			ExchangeType:    "topic",
			QueueTemplate:   "bastyle.replica.%s.events",
			RoutingKeys:     []string{"media.ban.#"},
			ReconnectDelay:  time.Second,
			CatchUpInterval: time.Second,
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Validate(); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}
