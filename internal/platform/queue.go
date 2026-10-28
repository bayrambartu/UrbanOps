package platform

import (
	"context"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"sync"
	"time"
)

const Queue = "urbanops.events"
const RetryQueue = "urbanops.events.retry"
const DeadQueue = "urbanops.events.dead"

type Broker struct {
	Conn    *amqp.Connection
	Ch      *amqp.Channel
	returns <-chan amqp.Return
	mu      sync.Mutex
}

func OpenBroker(url string) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	b := &Broker{Conn: conn, Ch: ch, returns: ch.NotifyReturn(make(chan amqp.Return, 1))}
	fail := func(err error) (*Broker, error) { b.Close(); return nil, err }
	if _, err = ch.QueueDeclare(DeadQueue, true, false, false, false, nil); err != nil {
		return fail(err)
	}
	if _, err = ch.QueueDeclare(RetryQueue, true, false, false, false, amqp.Table{"x-message-ttl": int32(5000), "x-dead-letter-exchange": "", "x-dead-letter-routing-key": Queue}); err != nil {
		return fail(err)
	}
	if _, err = ch.QueueDeclare(Queue, true, false, false, false, amqp.Table{"x-dead-letter-exchange": "", "x-dead-letter-routing-key": DeadQueue}); err != nil {
		return fail(err)
	}
	if err = ch.Confirm(false); err != nil {
		return fail(err)
	}
	return b, nil
}
func (b *Broker) Close() { b.Ch.Close(); b.Conn.Close() }
func (b *Broker) Publish(ctx context.Context, queue, id string, body []byte, headers amqp.Table) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	confirmation, err := b.Ch.PublishWithDeferredConfirmWithContext(ctx, "", queue, true, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: id, Body: body, Headers: headers, Timestamp: time.Now(),
	})
	if err != nil {
		return err
	}
	ok, err := confirmation.WaitContext(ctx)
	if err != nil {
		b.Conn.CloseDeadline(time.Now().Add(time.Second))
		return err
	}
	if !ok {
		return fmt.Errorf("broker mesajı onaylamadı")
	}
	select {
	case returned, ok := <-b.returns:
		if !ok {
			return amqp.ErrClosed
		}
		return fmt.Errorf("mesaj yönlendirilemedi: %s", returned.ReplyText)
	default:
	}
	return nil
}
