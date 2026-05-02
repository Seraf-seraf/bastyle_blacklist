package moderation

import (
	"github.com/Seraf-seraf/bastyle_blacklist/internal/app/ports"
	"github.com/Seraf-seraf/bastyle_blacklist/internal/domain"
)

type Service struct {
	contentMatcher ports.ContentMatcher
	admins         ports.AdminChecker
	actions        ports.MessageActions
}

func NewService(
	contentMatcher ports.ContentMatcher,
	admins ports.AdminChecker,
	actions ports.MessageActions,
) *Service {
	return &Service{
		contentMatcher: contentMatcher,
		admins:         admins,
		actions:        actions,
	}
}

func (s *Service) HandleMessage(msg domain.Message) error {
	if msg.IsCommand() {

		switch msg.Command {

		case "hello":
			if err := s.actions.SendMessage(msg.ChatID, "Hello, World!"); err != nil {
				return err
			}
		case "ban":
			isAdmin, err := s.admins.IsAdmin(msg.ChatID, msg.SenderID)
			if !isAdmin {
				_ = s.actions.SendMessage(msg.ChatID, "Команда доступна только админам")
				return err
			}

			if msg.ReplyTo == nil {
				_ = s.actions.SendMessage(msg.ChatID, "Reply на сообщение обязателен")
				return nil
			}

			target := msg.ReplyTo
			if target.Content != nil {
				if err := s.contentMatcher.Block(*target.Content); err != nil {
					return err
				}
			}

			if err := s.actions.DeleteMessage(target.ChatID, target.ID); err != nil {
				return err
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

		blocked, err := s.contentMatcher.IsBlocked(*target.Content)
		if err != nil {
			return err
		}

		if blocked {
			if err := s.actions.DeleteMessage(target.ChatID, target.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
