package moderation

import (
	"context"

	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/pkg/apperrors"
)

const privateChatInfo = "Bastyle Blacklist — бот для модерации медиа в групповых чатах.\nИсходники: https://github.com/Seraf-seraf/bastyle_blacklist"

type service struct {
	contentMatcher ports.ContentBlocker
	admins         ports.AdminChecker
	actions        ports.MessageActions
}

func NewService(
	contentMatcher ports.ContentBlocker,
	admins ports.AdminChecker,
	actions ports.MessageActions,
) (*service, error) {
	const methodCtx = "moderation/NewService"

	if contentMatcher == nil {
		return nil, apperrors.New(methodCtx, "матчер контента не настроен")
	}
	if admins == nil {
		return nil, apperrors.New(methodCtx, "проверка админов не настроена")
	}
	if actions == nil {
		return nil, apperrors.New(methodCtx, "действия с сообщениями не настроены")
	}

	return &service{
		contentMatcher: contentMatcher,
		admins:         admins,
		actions:        actions,
	}, nil
}

func (s *service) HandleMessage(ctx context.Context, msg domain.Message) error {
	const methodCtx = "moderation/service.HandleMessage"

	if msg.IsPrivateChat() {
		if err := s.actions.SendMessage(msg.ChatID, privateChatInfo); err != nil {
			return apperrors.Wrap(methodCtx, err)
		}
		return nil
	}

	if msg.IsCommand() {

		switch msg.Command {

		case "hello":
			if err := s.actions.SendMessage(msg.ChatID, "Hello, World!"); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
		case "ban":
			isAdmin, err := s.admins.IsAdmin(msg.ChatID, msg.SenderID)
			if !isAdmin {
				_ = s.actions.SendMessage(msg.ChatID, "Команда доступна только админам")
				return apperrors.Wrap(methodCtx, err)
			}

			if msg.ReplyTo == nil {
				_ = s.actions.SendMessage(msg.ChatID, "Reply на сообщение обязателен")
				return nil
			}

			target := msg.ReplyTo
			if target.Content == nil {
				_ = s.actions.SendMessage(msg.ChatID, "Текстовые сообщения не баним")
				return nil
			}

			if err := s.contentMatcher.Block(ctx, target.ChatID, *target.Content); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}

			if err := s.actions.DeleteMessage(target.ChatID, target.ID); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}

			if err := s.actions.DeleteMessage(msg.ChatID, msg.ID); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
		}

		return nil
	}

	targets := [2]*domain.Message{
		&msg, msg.ReplyTo,
	}

	for _, target := range targets {
		if target == nil {
			continue
		}

		if target.Content == nil {
			continue
		}

		blocked, err := s.contentMatcher.IsBlocked(ctx, target.ChatID, *target.Content)
		if err != nil {
			return apperrors.Wrap(methodCtx, err)
		}

		if blocked {
			if err := s.actions.DeleteMessage(target.ChatID, target.ID); err != nil {
				return apperrors.Wrap(methodCtx, err)
			}
		}
	}

	return nil
}
