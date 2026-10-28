package main

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"log/slog"
	"time"
	"urbanops/internal/domain"
	"urbanops/internal/platform"
)

func main() {
	ctx, stop := platform.Context()
	defer stop()
	db, err := pgxpool.New(ctx, platform.Required("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer db.Close()
	for ctx.Err() == nil {
		if err := run(ctx, db); err != nil {
			slog.Error("worker bağlantısı kesildi; yeniden bağlanıyor", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}
func run(ctx context.Context, db *pgxpool.Pool) error {
	b, err := platform.OpenBroker(platform.Required("RABBITMQ_URL"))
	if err != nil {
		return err
	}
	defer b.Close()
	consumer, err := b.Conn.Channel()
	if err != nil {
		return err
	}
	defer consumer.Close()
	if err = consumer.Qos(8, 0, false); err != nil {
		return err
	}
	msgs, err := consumer.Consume(platform.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	slog.Info("worker hazır")
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return amqp.ErrClosed
			}
			var e domain.Event
			err = json.Unmarshal(msg.Body, &e)
			if err == nil {
				err = e.Validate()
			}
			if err != nil {
				slog.Warn("geçersiz olay DLQ'ya taşındı", "error", err)
				if err = msg.Reject(false); err != nil {
					return err
				}
				continue
			}
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = platform.ApplyEvent(jobCtx, db, e)
			cancel()
			if err == nil {
				if err = msg.Ack(false); err != nil {
					return err
				}
				continue
			}
			attempts := int32(0)
			if n, ok := msg.Headers["x-retry"].(int32); ok {
				attempts = n
			}
			target := platform.RetryQueue
			if attempts >= 3 {
				target = platform.DeadQueue
			}
			slog.Warn("olay işlenemedi", "event_id", e.EventID, "retry", attempts, "error", err)
			retryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = b.Publish(retryCtx, target, e.EventID, msg.Body, amqp.Table{"x-retry": attempts + 1, "x-error": err.Error()})
			cancel()
			if err != nil {
				return err
			} // Channel close requeues the unacknowledged original.
			if err = msg.Ack(false); err != nil {
				return err
			}
		}
	}
}
