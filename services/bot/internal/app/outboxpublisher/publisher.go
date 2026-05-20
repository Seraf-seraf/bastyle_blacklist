package outboxpublisher

import (
	"context"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/components/forwarder"
	"github.com/ThreeDotsLabs/watermill/message"
)

type Config struct {
	ForwarderTopic string
	CloseTimeout   time.Duration
}

type Publisher struct {
	forwarder *forwarder.Forwarder
}

func New(
	subscriber message.Subscriber,
	publisher message.Publisher,
	logger watermill.LoggerAdapter,
	cfg Config,
) (*Publisher, error) {
	const methodCtx = "outboxpublisher/New"

	if subscriber == nil {
		return nil, apperrors.New(methodCtx, "SQL subscriber не настроен")
	}
	if publisher == nil {
		return nil, apperrors.New(methodCtx, "broker publisher не настроен")
	}
	if cfg.ForwarderTopic == "" {
		return nil, apperrors.New(methodCtx, "топик forwarder-а обязателен")
	}
	if cfg.CloseTimeout <= 0 {
		return nil, apperrors.New(methodCtx, "таймаут остановки forwarder-а должен быть положительным")
	}

	fwd, err := forwarder.NewForwarder(
		subscriber,
		publisher,
		logger,
		forwarder.Config{
			ForwarderTopic: cfg.ForwarderTopic,
			CloseTimeout:   cfg.CloseTimeout,
		},
	)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &Publisher{forwarder: fwd}, nil
}

func (p *Publisher) Run(ctx context.Context) error {
	const methodCtx = "outboxpublisher/Publisher.Run"

	return apperrors.Wrap(methodCtx, p.forwarder.Run(ctx))
}

func (p *Publisher) Close() error {
	return p.forwarder.Close()
}
