package bootstrap

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/database/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/httpclient"
	indexcheckpointpostgres "github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/indexcheckpoint/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/matching/orchestrator"
	outboxpostgres "github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/outbox/postgres"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/moderation"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/config"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ModerationService interface {
	HandleMessage(ctx context.Context, msg domain.Message) error
}

type Components struct {
	DBPool      *postgres.Pool
	Outbox      ports.OutboxStore
	Bot         *tgbotapi.BotAPI
	Updates     tgbotapi.UpdatesChannel
	Service     ModerationService
	ReplicaID   string
	Checkpoints ports.IndexCheckpointStore
	Appliers    []ports.IndexEventApplier
	Matchers    []ports.ContentBlockMatcher
}

func Build(ctx context.Context, cfg config.Config) (components Components, err error) {
	const methodCtx = "bootstrap/Build"

	dbPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}
	var matchers []ports.ContentBlockMatcher
	defer func() {
		if err == nil {
			return
		}
		_ = closeMatchers(matchers)
		dbPool.Close()
	}()

	outboxStore, err := outboxpostgres.NewStore(dbPool.Raw())
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}

	bot, err := newTelegramBot(cfg)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}
	log.Printf("Авторизован как %s", bot.Self.UserName)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = cfg.Telegram.UpdateTimeoutSeconds
	updates := bot.GetUpdatesChan(updateConfig)

	matchers, err = buildMatchers(ctx, cfg, dbPool, bot)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}

	contentMatcher, err := orchestrator.NewBlockOrchestrator(dbPool, outboxStore, matchers...)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}
	actions, err := telegram.NewBotActions(bot)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}
	admin, err := telegram.NewAdminChecker(bot)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}
	service, err := moderation.NewService(contentMatcher, admin, actions)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}

	replicaID, err := replicaID(cfg)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}

	checkpointStore, appliers, err := buildIndexDependencies(dbPool, cfg, matchers)
	if err != nil {
		return Components{}, apperrors.Wrap(methodCtx, err)
	}

	return Components{
		DBPool:      dbPool,
		Outbox:      outboxStore,
		Bot:         bot,
		Updates:     updates,
		Service:     service,
		ReplicaID:   replicaID,
		Checkpoints: checkpointStore,
		Appliers:    appliers,
		Matchers:    matchers,
	}, nil
}

type closeError interface {
	Close() error
}

func closeMatchers(matchers []ports.ContentBlockMatcher) error {
	var closeErr error
	for i := len(matchers) - 1; i >= 0; i-- {
		closer, ok := matchers[i].(closeError)
		if !ok {
			continue
		}
		closeErr = errors.Join(closeErr, closer.Close())
	}
	return closeErr
}

func newTelegramBot(cfg config.Config) (*tgbotapi.BotAPI, error) {
	const methodCtx = "bootstrap/newTelegramBot"

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

func replicaID(cfg config.Config) (string, error) {
	if !cfg.Consumers.IndexEvents.Enabled || cfg.Consumers.IndexEvents.ReplicaID != "" {
		return cfg.Consumers.IndexEvents.ReplicaID, nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return hostname, nil
}

func buildIndexDependencies(
	dbPool *postgres.Pool,
	cfg config.Config,
	matchers []ports.ContentBlockMatcher,
) (ports.IndexCheckpointStore, []ports.IndexEventApplier, error) {
	const methodCtx = "bootstrap/buildIndexDependencies"

	appliers := make([]ports.IndexEventApplier, 0, len(matchers))
	if !cfg.Consumers.IndexEvents.Enabled {
		return nil, appliers, nil
	}

	for _, matcher := range matchers {
		applier, ok := matcher.(ports.IndexEventApplier)
		if ok {
			appliers = append(appliers, applier)
		}
	}
	if len(appliers) == 0 {
		return nil, nil, apperrors.New(methodCtx, "нет index applier-ов для index_events consumer-а")
	}

	checkpointStore, err := indexcheckpointpostgres.NewStore(dbPool.Raw())
	if err != nil {
		return nil, nil, apperrors.Wrap(methodCtx, err)
	}
	return checkpointStore, appliers, nil
}
