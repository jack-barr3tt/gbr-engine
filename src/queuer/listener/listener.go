package listener

import (
	"context"
	"sync"

	"github.com/go-stomp/stomp/v3"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Listener struct {
	ctx       context.Context
	wg        *sync.WaitGroup
	channel   *amqp.Channel
	stompConn *stomp.Conn
	topic     string
	handler   func(*amqp.Channel, string)
}

func NewListener(ctx context.Context, wg *sync.WaitGroup, channel *amqp.Channel, stompConn *stomp.Conn, topic string, handler func(*amqp.Channel, string)) *Listener {
	return &Listener{
		ctx:       ctx,
		wg:        wg,
		channel:   channel,
		stompConn: stompConn,
		topic:     topic,
		handler:   handler,
	}
}

func (l *Listener) DeclareQueue(name string) error {
	_, err := l.channel.QueueDeclare(
		name,
		false,
		false,
		false,
		false,
		nil,
	)
	return err
}

func (l *Listener) Start() error {
	defer l.wg.Done()

	sub, err := l.stompConn.Subscribe(l.topic, stomp.AckAuto)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-l.ctx.Done():
			return nil
		case msg, ok := <-sub.C:
			if !ok {
				return nil
			}
			if msg.Err != nil {
				continue
			}

			l.handler(l.channel, string(msg.Body))
		}
	}
}
