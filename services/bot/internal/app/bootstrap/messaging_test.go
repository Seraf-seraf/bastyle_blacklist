package bootstrap

import (
	"context"
	"errors"
	"testing"
)

func TestStartIndexSubscriberRunsBootstrapBeforeSubscriber(t *testing.T) {
	order := make([]string, 0, 2)
	synchronizer := &fakeStartupSynchronizer{
		catchUp: func(context.Context) error {
			order = append(order, "catch-up")
			return nil
		},
	}
	subscriber := &fakeIndexSubscriberRunner{
		run: func(context.Context) error {
			order = append(order, "run")
			return nil
		},
	}

	indexSubscriber, err := newIndexSubscriber(context.Background(), synchronizer, subscriber)
	if err != nil {
		t.Fatal(err)
	}
	if err := indexSubscriber.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 || order[0] != "catch-up" || order[1] != "run" {
		t.Fatalf("неожиданный порядок запуска: %#v", order)
	}
}

func TestStartIndexSubscriberDoesNotRunSubscriberAfterBootstrapError(t *testing.T) {
	expectedErr := errors.New("catch-up failed")
	synchronizer := &fakeStartupSynchronizer{
		catchUp: func(context.Context) error {
			return expectedErr
		},
	}
	subscriber := &fakeIndexSubscriberRunner{
		run: func(context.Context) error {
			t.Fatal("subscriber не должен запускаться после ошибки bootstrap catch-up")
			return nil
		},
	}

	_, err := newIndexSubscriber(context.Background(), synchronizer, subscriber)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ожидалась ошибка bootstrap catch-up: %v", err)
	}
}

type fakeStartupSynchronizer struct {
	catchUp func(context.Context) error
}

func (s *fakeStartupSynchronizer) CatchUpAllIndexes(ctx context.Context) error {
	return s.catchUp(ctx)
}

type fakeIndexSubscriberRunner struct {
	run func(context.Context) error
}

func (r *fakeIndexSubscriberRunner) Run(ctx context.Context) error {
	return r.run(ctx)
}
