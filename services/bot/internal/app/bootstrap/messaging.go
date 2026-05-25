package bootstrap

import (
	"context"
	"errors"
	"log"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/messaging/rabbitmq"
	outboxpostgres "github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/outbox/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/indexsync"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/outboxpublisher"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/ThreeDotsLabs/watermill"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
)

type indexStartupSynchronizer interface {
	CatchUpAllIndexes(context.Context) error
}

type indexSubscriberRunner interface {
	Run(context.Context) error
}

type OutboxPublisher struct {
	sqlSubscriber   message.Subscriber
	rabbitPublisher message.Publisher
	publisher       *outboxpublisher.Publisher
}

func NewOutboxPublisher(cfg config.Config, dbPool *postgres.Pool) (*OutboxPublisher, error) {
	const methodCtx = "bootstrap/NewOutboxPublisher"

	if !cfg.OutboxPublisher.Enabled {
		return nil, nil
	}

	watermillLogger := watermill.NopLogger{}
	rabbitPublisher, err := rabbitmq.NewPublisher(rabbitmq.Config{
		URL:               cfg.RabbitMQ.URL,
		Exchange:          cfg.RabbitMQ.Exchange,
		ExchangeType:      cfg.RabbitMQ.ExchangeType,
		PublishTimeout:    cfg.RabbitMQ.PublishTimeout.Value(),
		ReconnectInterval: cfg.RabbitMQ.ReconnectInterval.Value(),
	}, watermillLogger)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	ackDeadline := cfg.RabbitMQ.PublishTimeout.Value()
	sqlSubscriber, err := watermillsql.NewSubscriber(watermillsql.BeginnerFromPgx(dbPool.Raw()), watermillsql.SubscriberConfig{
		ConsumerGroup:    outboxpostgres.ForwarderConsumerGroup,
		AckDeadline:      &ackDeadline,
		PollInterval:     cfg.OutboxPublisher.PollInterval.Value(),
		ResendInterval:   cfg.OutboxPublisher.RetryBaseDelay.Value(),
		RetryInterval:    cfg.OutboxPublisher.RetryBaseDelay.Value(),
		SchemaAdapter:    outboxpostgres.NewWatermillSchema(cfg.OutboxPublisher.BatchSize),
		OffsetsAdapter:   outboxpostgres.NewWatermillOffsetsAdapter(),
		InitializeSchema: false,
	}, watermillLogger)
	if err != nil {
		_ = rabbitPublisher.Close()
		return nil, apperrors.Wrap(methodCtx, err)
	}

	publisher, err := outboxpublisher.New(sqlSubscriber, rabbitPublisher, watermillLogger, outboxpublisher.Config{
		ForwarderTopic: outboxpostgres.ForwarderTopic,
		CloseTimeout:   cfg.OutboxPublisher.LockTTL.Value(),
	})
	if err != nil {
		_ = sqlSubscriber.Close()
		_ = rabbitPublisher.Close()
		return nil, apperrors.Wrap(methodCtx, err)
	}

	return &OutboxPublisher{
		sqlSubscriber:   sqlSubscriber,
		rabbitPublisher: rabbitPublisher,
		publisher:       publisher,
	}, nil
}

func (p *OutboxPublisher) Run(ctx context.Context) error {
	return p.publisher.Run(ctx)
}

func (p *OutboxPublisher) Close() error {
	var closeErr error
	if p.publisher != nil {
		closeErr = errors.Join(closeErr, p.publisher.Close())
	}
	if p.sqlSubscriber != nil {
		closeErr = errors.Join(closeErr, p.sqlSubscriber.Close())
	}
	if p.rabbitPublisher != nil {
		closeErr = errors.Join(closeErr, p.rabbitPublisher.Close())
	}
	return closeErr
}

type IndexSubscriber struct {
	subscriber indexSubscriberRunner
}

func NewIndexSubscriber(
	ctx context.Context,
	cfg config.Config,
	dbPool *postgres.Pool,
	replicaID string,
	checkpointStore ports.IndexCheckpointStore,
	appliers []ports.IndexEventApplier,
) (*IndexSubscriber, error) {
	const methodCtx = "bootstrap/NewIndexSubscriber"

	if !cfg.Consumers.IndexEvents.Enabled {
		return nil, nil
	}

	eventReader, err := outboxpostgres.NewEventReader(dbPool.Raw())
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	synchronizer, err := indexsync.New(indexsync.Config{
		ConsumerID:  replicaID,
		BatchSize:   cfg.Consumers.IndexEvents.CatchUpBatchSize,
		Reader:      eventReader,
		Checkpoints: checkpointStore,
		Appliers:    appliers,
	})
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	indexSubscriber, err := rabbitmq.NewIndexSubscriber(rabbitmq.IndexSubscriberConfig{
		URL:             cfg.RabbitMQ.URL,
		Exchange:        cfg.RabbitMQ.Exchange,
		ExchangeType:    cfg.RabbitMQ.ExchangeType,
		ReplicaID:       replicaID,
		QueueTemplate:   cfg.Consumers.IndexEvents.QueueTemplate,
		RoutingKeys:     cfg.Consumers.IndexEvents.RoutingKeys,
		Prefetch:        cfg.Consumers.IndexEvents.Prefetch,
		ReconnectDelay:  cfg.RabbitMQ.ReconnectInterval.Value(),
		CatchUpInterval: cfg.Consumers.IndexEvents.CatchUpInterval.Value(),
	}, synchronizer)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	return newIndexSubscriber(ctx, synchronizer, indexSubscriber)
}

func newIndexSubscriber(
	ctx context.Context,
	synchronizer indexStartupSynchronizer,
	subscriber indexSubscriberRunner,
) (*IndexSubscriber, error) {
	const methodCtx = "bootstrap/newIndexSubscriber"

	if synchronizer == nil {
		return nil, apperrors.New(methodCtx, "index synchronizer не настроен")
	}
	if subscriber == nil {
		return nil, apperrors.New(methodCtx, "index subscriber не настроен")
	}

	log.Println("Начинается bootstrap catch-up локальных индексов")
	if err := synchronizer.CatchUpAllIndexes(ctx); err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}
	log.Println("Bootstrap catch-up локальных индексов завершен")

	return &IndexSubscriber{subscriber: subscriber}, nil
}

func (s *IndexSubscriber) Run(ctx context.Context) error {
	return s.subscriber.Run(ctx)
}
