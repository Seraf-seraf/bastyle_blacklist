package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/httpclient"
	indexcheckpointpostgres "github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/indexcheckpoint/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/aivector"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/exact"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/imagehash"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/orchestrator"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/videolike"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/media"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/messaging/rabbitmq"
	outboxpostgres "github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/outbox/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/indexsync"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/moderation"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/outboxpublisher"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
	"github.com/ThreeDotsLabs/watermill"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Job struct {
	Update tgbotapi.Update
}

type indexStartupSynchronizer interface {
	CatchUpAllIndexes(context.Context) error
}

type indexSubscriberRunner interface {
	Run(context.Context) error
}

func main() {
	const methodCtx = "cmd/main"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	configPath := flag.String("config", "infra/config/config.yaml", "путь к YAML-конфигу")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	defer func() {
		dbPool.Close()
		log.Println("PostgreSQL pool закрыт")
	}()
	outboxStore, err := outboxpostgres.NewStore(dbPool.Raw())
	if err != nil {
		logging.Panic(methodCtx, err)
	}

	bot, err := newTelegramBot(cfg)
	if err != nil {
		logging.Panic(methodCtx, err)
	}

	log.Printf("Авторизован как %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = cfg.Telegram.UpdateTimeoutSeconds

	updates := bot.GetUpdatesChan(u)
	go func() {
		<-ctx.Done()
		log.Println("Получен сигнал завершения")
		bot.StopReceivingUpdates()
	}()

	exactMatcher, err := exact.NewPostgresMatcher(ctx, dbPool.Raw(), cfg.Matching.Exact.Buffer)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	mediaDownloader, err := telegram.NewFileDownloader(bot)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	mediaExtractor := media.NewExtractor()
	imageHashMatcher, err := imagehash.NewPostgresMatcher(
		ctx,
		dbPool.Raw(),
		mediaDownloader,
		mediaExtractor,
		cfg.Matching.ImageHash.Threshold,
		cfg.Matching.ImageHash.Buffer,
	)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	defer func() {
		if err := imageHashMatcher.Close(); err != nil {
			logging.Error(methodCtx, err)
		}
	}()

	defer func() {
		if err := exactMatcher.Close(); err != nil {
			logging.Error(methodCtx, err)
		}
	}()

	matchers := []ports.ContentBlockMatcher{exactMatcher, imageHashMatcher}
	videoLikeExtractor, err := media.NewFFmpegFrameExtractor(
		cfg.MediaConfig.FFmpegBinary,
		cfg.MediaConfig.FFmpegTimeout.Value(),
	)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	videoLikeMatcher, err := videolike.NewPostgresMatcher(
		ctx,
		dbPool.Raw(),
		mediaDownloader,
		videoLikeExtractor,
		cfg.Matching.VideoLike.Threshold,
		cfg.Matching.VideoLike.Buffer,
		domain.MediaExtractionPlan{
			MaxFrames:    cfg.MediaConfig.MaxFrames,
			TargetWidth:  cfg.MediaConfig.TargetWidth,
			TargetHeight: cfg.MediaConfig.TargetHeight,
		},
		videolike.Limits{
			MaxAnimationDuration:    cfg.MediaConfig.MaxAnimationDuration.Value(),
			MaxVideoStickerDuration: cfg.MediaConfig.MaxVideoStickerDuration.Value(),
			MaxAnimationSize:        cfg.MediaConfig.MaxAnimationSize.Bytes(),
			MaxVideoStickerSize:     cfg.MediaConfig.MaxVideoStickerSize.Bytes(),
		},
		videolike.MatchRule{
			MinMatchedFrames: cfg.Matching.VideoLike.MinMatchedFrames,
			MinMatchedRatio:  cfg.Matching.VideoLike.MinMatchedRatio,
		},
	)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	defer func() {
		if err := videoLikeMatcher.Close(); err != nil {
			logging.Error(methodCtx, err)
		}
	}()
	matchers = append(matchers, videoLikeMatcher)

	if cfg.Matching.AIVector.Enabled {
		aiVectorClient, err := aivector.NewHTTPClient(
			cfg.Matching.AIVector.Service.URL(),
			cfg.Matching.AIVector.RequestTimeout.Value(),
		)
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		aiVectorExtractor, err := media.NewFFmpegFrameExtractor(
			cfg.MediaConfig.FFmpegBinary,
			cfg.MediaConfig.FFmpegTimeout.Value(),
		)
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		aiVectorMatcher, err := aivector.NewMatcher(aivector.Options{
			Downloader:          mediaDownloader,
			ImageFrameExtractor: mediaExtractor,
			VideoFrameExtractor: aiVectorExtractor,
			Client:              aiVectorClient,
			ModelName:           cfg.Matching.AIVector.ModelName,
			ModelRevision:       cfg.Matching.AIVector.ModelRevision,
			Threshold:           cfg.Matching.AIVector.Threshold,
			TopK:                cfg.Matching.AIVector.TopK,
			Plan: domain.MediaExtractionPlan{
				MaxFrames:    cfg.MediaConfig.MaxFrames,
				TargetWidth:  cfg.MediaConfig.TargetWidth,
				TargetHeight: cfg.MediaConfig.TargetHeight,
			},
			Limits: aivector.Limits{
				MaxAnimationDuration:    cfg.MediaConfig.MaxAnimationDuration.Value(),
				MaxVideoStickerDuration: cfg.MediaConfig.MaxVideoStickerDuration.Value(),
				MaxAnimationSize:        cfg.MediaConfig.MaxAnimationSize.Bytes(),
				MaxVideoStickerSize:     cfg.MediaConfig.MaxVideoStickerSize.Bytes(),
			},
			Rule: aivector.MatchRule{
				MinMatchedFrames: cfg.Matching.AIVector.MinMatchedFrames,
				MinMatchedRatio:  cfg.Matching.AIVector.MinMatchedRatio,
			},
		})
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		matchers = append(matchers, aiVectorMatcher)
	}

	contentMatcher, err := orchestrator.NewBlockOrchestrator(dbPool, outboxStore, matchers...)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	actions, err := telegram.NewBotActions(bot)
	if err != nil {
		logging.Panic(methodCtx, err)
	}
	admin, err := telegram.NewAdminChecker(bot)
	if err != nil {
		logging.Panic(methodCtx, err)
	}

	service, err := moderation.NewService(contentMatcher, admin, actions)
	if err != nil {
		logging.Panic(methodCtx, err)
	}

	replicaID := cfg.Consumers.IndexEvents.ReplicaID
	if cfg.Consumers.IndexEvents.Enabled && replicaID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		replicaID = hostname
	}

	var checkpointStore ports.IndexCheckpointStore
	appliers := make([]ports.IndexEventApplier, 0, len(matchers))
	if cfg.Consumers.IndexEvents.Enabled {
		for _, matcher := range matchers {
			applier, ok := matcher.(ports.IndexEventApplier)
			if ok {
				appliers = append(appliers, applier)
			}
		}
		if len(appliers) == 0 {
			logging.Panic(methodCtx, apperrors.New(methodCtx, "нет index applier-ов для index_events consumer-а"))
		}
		checkpointStore, err = indexcheckpointpostgres.NewStore(dbPool.Raw())
		if err != nil {
			logging.Panic(methodCtx, err)
		}
	}

	if cfg.Health.Enabled {
		healthServer, err := startHealthServer(ctx, cfg.Health.Address(), readinessProbe(dbPool.Ping, checkpointStore, replicaID, appliers))
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				logging.Error(methodCtx, err)
			}
		}()
	}

	if cfg.OutboxPublisher.Enabled {
		watermillLogger := watermill.NopLogger{}
		rabbitPublisher, err := rabbitmq.NewPublisher(rabbitmq.Config{
			URL:               cfg.RabbitMQ.URL,
			Exchange:          cfg.RabbitMQ.Exchange,
			ExchangeType:      cfg.RabbitMQ.ExchangeType,
			PublishTimeout:    cfg.RabbitMQ.PublishTimeout.Value(),
			ReconnectInterval: cfg.RabbitMQ.ReconnectInterval.Value(),
		}, watermillLogger)
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		defer func() {
			if err := rabbitPublisher.Close(); err != nil {
				logging.Error(methodCtx, err)
			}
		}()

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
			logging.Panic(methodCtx, err)
		}
		defer func() {
			if err := sqlSubscriber.Close(); err != nil {
				logging.Error(methodCtx, err)
			}
		}()

		publisher, err := outboxpublisher.New(sqlSubscriber, rabbitPublisher, watermillLogger, outboxpublisher.Config{
			ForwarderTopic: outboxpostgres.ForwarderTopic,
			CloseTimeout:   cfg.OutboxPublisher.LockTTL.Value(),
		})
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		defer func() {
			if err := publisher.Close(); err != nil {
				logging.Error(methodCtx, err)
			}
		}()

		go func() {
			if err := publisher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logging.Error(methodCtx, err)
			}
		}()
		log.Println("Watermill outbox publisher запущен")
	}

	if cfg.Consumers.IndexEvents.Enabled {
		eventReader, err := outboxpostgres.NewEventReader(dbPool.Raw())
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		synchronizer, err := indexsync.New(indexsync.Config{
			ConsumerID:  replicaID,
			BatchSize:   cfg.Consumers.IndexEvents.CatchUpBatchSize,
			Reader:      eventReader,
			Checkpoints: checkpointStore,
			Appliers:    appliers,
		})
		if err != nil {
			logging.Panic(methodCtx, err)
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
			logging.Panic(methodCtx, err)
		}
		if err := startIndexSubscriber(ctx, synchronizer, indexSubscriber); err != nil {
			logging.Panic(methodCtx, err)
		}
	}

	if cfg.Metrics.Enabled {
		metricsServer, err := startMetricsServer(ctx, cfg.Metrics, outboxStore)
		if err != nil {
			logging.Panic(methodCtx, err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				logging.Error(methodCtx, err)
			}
		}()
	}

	jobs := make(chan Job, cfg.JobsBuffer)
	var wg sync.WaitGroup

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(ctx, jobs, service)
		}()
	}

	for update := range updates {
		select {
		case <-ctx.Done():
			log.Println("Остановка приема обновлений")
			close(jobs)
			wg.Wait()
			log.Println("Завершение работы выполнено")
			return
		case jobs <- Job{Update: update}:
		}
	}

	close(jobs)
	wg.Wait()
	log.Println("Завершение работы выполнено")
}

type readinessCheck func(context.Context) error

func readinessProbe(dbPing readinessCheck, checkpoints ports.IndexCheckpointStore, consumerID string, appliers []ports.IndexEventApplier) readinessCheck {
	return func(ctx context.Context) error {
		const methodCtx = "cmd/readinessProbe"

		if dbPing != nil {
			if err := dbPing(ctx); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
		}
		if checkpoints == nil {
			return nil
		}

		indexNames := make([]string, 0, len(appliers))
		for _, applier := range appliers {
			indexNames = append(indexNames, applier.IndexName())
		}
		return apperrors.Wrap(methodCtx, checkpoints.CheckFresh(ctx, consumerID, indexNames))
	}
}

func startIndexSubscriber(ctx context.Context, synchronizer indexStartupSynchronizer, subscriber indexSubscriberRunner) error {
	const methodCtx = "cmd/startIndexSubscriber"

	if synchronizer == nil {
		return apperrors.New(methodCtx, "index synchronizer не настроен")
	}
	if subscriber == nil {
		return apperrors.New(methodCtx, "index subscriber не настроен")
	}

	log.Println("Начинается bootstrap catch-up локальных индексов")
	if err := synchronizer.CatchUpAllIndexes(ctx); err != nil {
		return apperrors.Wrap(methodCtx, err)
	}
	log.Println("Bootstrap catch-up локальных индексов завершен")

	go func() {
		if err := subscriber.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.Error(methodCtx, err)
		}
	}()
	log.Println("RabbitMQ index events consumer запущен")
	return nil
}

func startHealthServer(ctx context.Context, address string, readiness readinessCheck) (*http.Server, error) {
	const methodCtx = "cmd/startHealthServer"

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeReadinessStatus(ctx, w, readiness)
	})

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logging.Error(methodCtx, err)
		}
	}()

	go func() {
		log.Printf("Health-сервер слушает %s", address)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Error(methodCtx, err)
		}
	}()

	return server, nil
}

func writeReadinessStatus(ctx context.Context, w http.ResponseWriter, readiness readinessCheck) {
	const methodCtx = "cmd/writeReadinessStatus"

	if readiness != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := readiness(checkCtx); err != nil {
			logging.Error(methodCtx, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func newTelegramBot(cfg config.Config) (*tgbotapi.BotAPI, error) {
	const methodCtx = "cmd/newTelegramBot"

	if !cfg.Telegram.HTTPClient.Enabled {
		bot, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
		return bot, apperrors.Wrap(methodCtx, err)
	}

	client, err := httpclient.New(httpclient.Options{
		ProxyURL: cfg.Telegram.HTTPClient.ProxyURL,
	})
	if err != nil {
		return nil, apperrors.Wrap(methodCtx, err)
	}

	bot, err := tgbotapi.NewBotAPIWithClient(cfg.Telegram.Token, tgbotapi.APIEndpoint, client)
	return bot, apperrors.Wrap(methodCtx, err)
}

type moderationService interface {
	HandleMessage(ctx context.Context, msg domain.Message) error
}

func worker(ctx context.Context, jobs <-chan Job, service moderationService) {
	const methodCtx = "cmd/worker"

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			msg := job.Update.Message
			if msg == nil {
				continue
			}

			message := telegram.MessageFromTelegram(msg)
			if err := service.HandleMessage(ctx, message); err != nil {
				logging.Error(methodCtx, err)
			}
		}
	}
}
