package runtime

import (
	"context"
	"log"
	"sync"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/adapters/telegram"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/logging"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type moderationService interface {
	HandleMessage(ctx context.Context, msg domain.Message) error
}

type job struct {
	Update tgbotapi.Update
}

func StartTelegramWorkers(
	ctx context.Context,
	wg *sync.WaitGroup,
	updates tgbotapi.UpdatesChannel,
	workers int,
	jobsBuffer int,
	service moderationService,
) {
	if wg == nil {
		return
	}

	jobs := make(chan job, jobsBuffer)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(ctx, jobs, service)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(jobs)
		for update := range updates {
			select {
			case <-ctx.Done():
				log.Println("Остановка приема обновлений")
				return
			case jobs <- job{Update: update}:
			}
		}
	}()
}

func worker(ctx context.Context, jobs <-chan job, service moderationService) {
	const methodCtx = "runtime/worker"

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
