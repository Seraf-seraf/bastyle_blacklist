//go:build integration

package rabbitmq

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestIndexSubscriberConsumesSignalAndRunsCatchUp(t *testing.T) {
	url := os.Getenv("RABBITMQ_TEST_URL")
	if url == "" {
		t.Skip("RABBITMQ_TEST_URL не задан")
	}

	sync := &countingSynchronizer{}
	cfg := IndexSubscriberConfig{
		URL:             url,
		Exchange:        "bastyle.events.test",
		ExchangeType:    "topic",
		ReplicaID:       "integration-replica",
		QueueTemplate:   "bastyle.replica.%s.events",
		RoutingKeys:     []string{"media.ban.#", "index.#"},
		Prefetch:        1,
		ReconnectDelay:  time.Second,
		CatchUpInterval: time.Hour,
	}
	subscriber, err := NewIndexSubscriber(cfg, sync)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- subscriber.Run(ctx)
	}()

	time.Sleep(700 * time.Millisecond)

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	if err := ch.PublishWithContext(ctx, cfg.Exchange, "media.ban.created.v1", false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers: amqp.Table{
			"_watermill_message_uuid": "integration-message",
		},
		Body: []byte(`{"signal":true}`),
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for sync.count.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("subscriber не выполнил catch-up после RabbitMQ-сигнала")
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("subscriber не завершился после отмены context")
	}
}

type countingSynchronizer struct {
	count atomic.Int64
}

func (s *countingSynchronizer) CatchUpAllIndexes(context.Context) error {
	s.count.Add(1)
	return nil
}
