package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	amqp "github.com/rabbitmq/amqp091-go"
)

type IndexSubscriberConfig struct {
	URL             string
	Exchange        string
	ExchangeType    string
	ReplicaID       string
	QueueTemplate   string
	RoutingKeys     []string
	Prefetch        int
	ReconnectDelay  time.Duration
	CatchUpInterval time.Duration
}

type IndexSynchronizer interface {
	CatchUpAllIndexes(ctx context.Context) error
}

type indexSubscriber struct {
	cfg          IndexSubscriberConfig
	synchronizer IndexSynchronizer
}

func NewIndexSubscriber(cfg IndexSubscriberConfig, synchronizer IndexSynchronizer) (*indexSubscriber, error) {
	const methodCtx = "rabbitmq/NewIndexSubscriber"

	if err := cfg.Validate(); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	if synchronizer == nil {
		return nil, apperrors.New(methodCtx, "index synchronizer не настроен")
	}

	return &indexSubscriber{cfg: cfg, synchronizer: synchronizer}, nil
}

func (s *indexSubscriber) Run(ctx context.Context) error {
	const methodCtx = "rabbitmq/IndexSubscriber.Run"

	queueName, err := s.cfg.QueueName()
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	conn, err := amqp.Dial(s.cfg.URL)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = ch.Close()
	}()

	if err := ch.ExchangeDeclare(s.cfg.Exchange, s.cfg.ExchangeType, true, false, false, false, nil); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, amqp.Table{"x-queue-type": "quorum"}); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	for _, routingKey := range s.cfg.RoutingKeys {
		if err := ch.QueueBind(queueName, routingKey, s.cfg.Exchange, false, nil); err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
	}
	if err := ch.Qos(s.cfg.Prefetch, 0, false); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	deliveries, err := ch.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}

	ticker := time.NewTicker(s.cfg.CatchUpInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.synchronizer.CatchUpAllIndexes(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Ошибка periodic catch-up индексов: %v", err)
			}
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}
			if err := s.synchronizer.CatchUpAllIndexes(ctx); err != nil {
				_ = delivery.Nack(false, true)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

func (c IndexSubscriberConfig) Validate() error {
	const methodCtx = "rabbitmq/IndexSubscriberConfig.Validate"

	if c.URL == "" {
		return apperrors.New(methodCtx, "RabbitMQ URL обязателен")
	}
	if c.Exchange == "" {
		return apperrors.New(methodCtx, "RabbitMQ exchange обязателен")
	}
	if c.ExchangeType == "" {
		return apperrors.New(methodCtx, "тип RabbitMQ exchange обязателен")
	}
	if c.QueueTemplate == "" {
		return apperrors.New(methodCtx, "шаблон очереди RabbitMQ обязателен")
	}
	if len(c.RoutingKeys) == 0 {
		return apperrors.New(methodCtx, "routing keys RabbitMQ обязательны")
	}
	for _, routingKey := range c.RoutingKeys {
		if routingKey == "" {
			return apperrors.New(methodCtx, "routing key RabbitMQ не должен быть пустым")
		}
	}
	if c.Prefetch <= 0 {
		return apperrors.New(methodCtx, "prefetch RabbitMQ должен быть положительным")
	}
	if c.ReconnectDelay <= 0 {
		return apperrors.New(methodCtx, "интервал reconnect RabbitMQ должен быть положительным")
	}
	if c.CatchUpInterval <= 0 {
		return apperrors.New(methodCtx, "catch-up interval должен быть положительным")
	}
	return nil
}

func (c IndexSubscriberConfig) QueueName() (string, error) {
	const methodCtx = "rabbitmq/IndexSubscriberConfig.QueueName"

	replicaID := c.ReplicaID
	if replicaID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return "", apperrors.Wrap(methodCtx, err)
		}
		replicaID = hostname
	}
	if replicaID == "" {
		return "", apperrors.New(methodCtx, "replica_id обязателен")
	}
	if c.QueueTemplate == "" {
		return "", apperrors.New(methodCtx, "шаблон очереди RabbitMQ обязателен")
	}
	return fmt.Sprintf(c.QueueTemplate, replicaID), nil
}
