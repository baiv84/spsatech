package push

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"spsatech/helpdesk/internal/store"
)

const maxAttempts = 5

// Worker раз в несколько секунд разбирает очередь push_outbox.
// Если FCM не настроен (sender == nil), уведомления помечаются отправленными,
// чтобы очередь не росла.
type Worker struct {
	store  *store.Store
	sender *FCM
}

func NewWorker(st *store.Store, sender *FCM) *Worker {
	return &Worker{store: st, sender: sender}
}

func (w *Worker) Start(ctx context.Context) {
	go func() {
		tick := time.NewTicker(3 * time.Second)
		cleanup := time.NewTicker(time.Hour)
		defer tick.Stop()
		defer cleanup.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := w.processBatch(ctx); err != nil && ctx.Err() == nil {
					slog.Error("push queue", "err", err)
				}
			case <-cleanup.C:
				w.store.CleanupPushes(ctx)
			}
		}
	}()
}

func (w *Worker) processBatch(ctx context.Context) error {
	jobs, err := w.store.PendingPushes(ctx, maxAttempts, 50)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := w.deliver(ctx, job); err != nil {
			slog.Warn("push delivery failed", "id", job.ID, "attempt", job.Attempts+1, "err", err)
			w.store.MarkPushFailed(ctx, job.ID, err.Error())
			continue
		}
		w.store.MarkPushSent(ctx, job.ID)
	}
	return nil
}

func (w *Worker) deliver(ctx context.Context, job store.PushJob) error {
	if w.sender == nil {
		return nil
	}
	tokens, err := w.store.DeviceTokens(ctx, job.UserID)
	if err != nil {
		return err
	}
	data := map[string]string{
		"ticketId": strconv.FormatInt(job.TicketID, 10),
		"title":    job.Title,
		"body":     job.Body,
	}
	var lastErr error
	for _, t := range tokens {
		err := w.sender.Send(ctx, t, data)
		switch {
		case errors.Is(err, ErrInvalidToken):
			w.store.DeleteDeviceToken(ctx, t)
		case err != nil:
			lastErr = err
		}
	}
	return lastErr
}
