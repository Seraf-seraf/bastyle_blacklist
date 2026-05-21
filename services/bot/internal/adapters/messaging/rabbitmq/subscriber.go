package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/ThreeDotsLabs/watermill"
	watermillamqp "github.com/ThreeDotsLabs/watermill-amqp/v3/pkg/amqp"
	"github.com/ThreeDotsLabs/watermill/message"
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

	watermillConfig, err := s.watermillConfig()
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	subscriber, err := watermillamqp.NewSubscriber(watermillConfig, watermill.NopLogger{})
	if err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	defer func() {
		_ = subscriber.Close()
	}()

	messages, err := s.subscribeRoutingKeys(ctx, subscriber)
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
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			if err := s.synchronizer.CatchUpAllIndexes(ctx); err != nil {
				msg.Nack()
				continue
			}
			msg.Ack()
		}
	}
}

func (s *indexSubscriber) subscribeRoutingKeys(ctx context.Context, subscriber *watermillamqp.Subscriber) (<-chan *message.Message, error) {
	const methodCtx = "rabbitmq/IndexSubscriber.subscribeRoutingKeys"

	for _, routingKey := range s.cfg.RoutingKeys[1:] {
		if err := subscriber.SubscribeInitialize(routingKey); err != nil {
			return nil, apperrors.Wrap(methodCtx, err)
		}
	}
	messages, err := subscriber.Subscribe(ctx, s.cfg.RoutingKeys[0])
	return messages, apperrors.Wrap(methodCtx, err)
}

func (s *indexSubscriber) watermillConfig() (watermillamqp.Config, error) {
	const methodCtx = "rabbitmq/IndexSubscriber.watermillConfig"

	queueName, err := s.cfg.QueueName()
	if err != nil {
		return watermillamqp.Config{}, apperrors.Wrap(methodCtx, err)
	}

	return watermillamqp.Config{
		Connection: watermillamqp.ConnectionConfig{
			AmqpURI: s.cfg.URL,
			Reconnect: &watermillamqp.ReconnectConfig{
				BackoffInitialInterval:     s.cfg.ReconnectDelay,
				BackoffRandomizationFactor: 0.2,
				BackoffMultiplier:          1.5,
				BackoffMaxInterval:         s.cfg.ReconnectDelay * 6,
			},
		},
		Marshaler: watermillamqp.DefaultMarshaler{},
		Exchange: watermillamqp.ExchangeConfig{
			GenerateName: watermillamqp.GenerateExchangeNameConstant(s.cfg.Exchange),
			Type:         s.cfg.ExchangeType,
			Durable:      true,
		},
		Queue: watermillamqp.QueueConfig{
			GenerateName: watermillamqp.GenerateQueueNameConstant(queueName),
			Durable:      true,
			Exclusive:    false,
			AutoDelete:   false,
			Arguments:    amqp.Table{"x-queue-type": "quorum"},
		},
		QueueBind: watermillamqp.QueueBindConfig{
			GenerateRoutingKey: func(topic string) string {
				return topic
			},
		},
		Consume: watermillamqp.ConsumeConfig{
			NoRequeueOnNack: false,
			Qos: watermillamqp.QosConfig{
				PrefetchCount: s.cfg.Prefetch,
			},
		},
		TopologyBuilder: &watermillamqp.DefaultTopologyBuilder{},
	}, nil
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
