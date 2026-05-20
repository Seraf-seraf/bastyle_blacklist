package rabbitmq

import (
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/ThreeDotsLabs/watermill"
	watermillamqp "github.com/ThreeDotsLabs/watermill-amqp/v3/pkg/amqp"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	URL               string
	Exchange          string
	ExchangeType      string
	PublishTimeout    time.Duration
	ReconnectInterval time.Duration
}

func NewPublisher(cfg Config, logger watermill.LoggerAdapter) (*watermillamqp.Publisher, error) {
	const methodCtx = "rabbitmq/NewPublisher"

	if cfg.URL == "" {
		return nil, apperrors.New(methodCtx, "RabbitMQ URL обязателен")
	}
	if cfg.Exchange == "" {
		return nil, apperrors.New(methodCtx, "RabbitMQ exchange обязателен")
	}
	if cfg.ExchangeType == "" {
		return nil, apperrors.New(methodCtx, "тип RabbitMQ exchange обязателен")
	}
	if cfg.PublishTimeout <= 0 {
		return nil, apperrors.New(methodCtx, "таймаут публикации RabbitMQ должен быть положительным")
	}
	if cfg.ReconnectInterval <= 0 {
		return nil, apperrors.New(methodCtx, "интервал reconnect RabbitMQ должен быть положительным")
	}

	publisher, err := watermillamqp.NewPublisher(watermillamqp.Config{
		Connection: watermillamqp.ConnectionConfig{
			AmqpURI: cfg.URL,
			Reconnect: &watermillamqp.ReconnectConfig{
				BackoffInitialInterval:     cfg.ReconnectInterval,
				BackoffRandomizationFactor: 0.2,
				BackoffMultiplier:          1.5,
				BackoffMaxInterval:         cfg.ReconnectInterval * 6,
			},
		},
		Marshaler: watermillamqp.DefaultMarshaler{
			PostprocessPublishing: func(publishing amqp.Publishing) amqp.Publishing {
				if messageID, ok := publishing.Headers[watermillamqp.DefaultMessageUUIDHeaderKey].(string); ok {
					publishing.MessageId = messageID
				}
				if eventType, ok := publishing.Headers["event_type"].(string); ok {
					publishing.Type = eventType
				}
				publishing.ContentType = "application/json"
				publishing.Timestamp = time.Now().UTC()
				return publishing
			},
		},
		Exchange: watermillamqp.ExchangeConfig{
			GenerateName: watermillamqp.GenerateExchangeNameConstant(cfg.Exchange),
			Type:         cfg.ExchangeType,
			Durable:      true,
		},
		Publish: watermillamqp.PublishConfig{
			GenerateRoutingKey: func(topic string) string {
				return topic
			},
			ConfirmDelivery: true,
		},
		TopologyBuilder: &watermillamqp.DefaultTopologyBuilder{},
	}, logger)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return publisher, nil
}
