package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jack-barr3tt/gbr-engine/src/queuer/listener"

	amqp "github.com/rabbitmq/amqp091-go"
)

var mqConn *amqp.Connection

func HandleTrust(channel *amqp.Channel, data string) {
	messages, err := utils.UnmarshalTrustMessages(data)
	if err != nil {
		utils.GetLogger().Warnw("error unmarshalling TRUST message", "error", err)
		return
	}

	for _, message := range messages {
		body, _ := json.Marshal(message)
		err = channel.Publish(
			"",
			"trust",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		if err != nil {
			utils.GetLogger().Warnw("error publishing message to RabbitMQ", "queue", "trust", "error", err)
		} else {
			utils.GetLogger().Debug("Published message to RabbitMQ for TRUST")
		}
	}
}

func HandleTD(channel *amqp.Channel, data string) {
	tdcMessages, tdsMessages, err := utils.UnmarshalTDMessages(data)
	if err != nil {
		utils.GetLogger().Warnw("error unmarshalling TD message", "error", err)
		return
	}

	for _, message := range tdcMessages {
		body, _ := json.Marshal(message)
		err = channel.Publish(
			"",
			"tdc",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		if err != nil {
			utils.GetLogger().Warnw("error publishing message to RabbitMQ", "queue", "tdc", "error", err)
		} else {
			utils.GetLogger().Debug("Published message to RabbitMQ for TD-C")
		}
	}

	for _, message := range tdsMessages {
		body, _ := json.Marshal(message)
		err = channel.Publish(
			"",
			"tds",
			false,
			false,
			amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			},
		)
		if err != nil {
			utils.GetLogger().Warnw("error publishing message to RabbitMQ", "queue", "tds", "error", err)
		} else {
			utils.GetLogger().Debug("Published message to RabbitMQ for TD-S")
		}
	}
}

func HandleVSTP(channel *amqp.Channel, data string) {
	message, err := utils.UnmarshalVSTP(data)
	if err != nil {
		utils.GetLogger().Warnw("error unmarshalling VSTP message", "error", err)
		return
	}

	body, _ := json.Marshal(message)
	err = channel.Publish(
		"",
		"vstp",
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		utils.GetLogger().Warnw("error publishing message to RabbitMQ", "queue", "vstp", "error", err)
	} else {
		utils.GetLogger().Debug("Published message to RabbitMQ for VSTP")
	}
}

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	logger := utils.GetLogger()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	mqConn, err = utils.NewRabbitConnectionOnly()
	if err != nil {
		logger.Fatalw("failed to connect to RabbitMQ", "error", err)
	}
	defer mqConn.Close()

	closeChan := make(chan *amqp.Error)
	mqConn.NotifyClose(closeChan)

	go func() {
		select {
		case err := <-closeChan:
			if err != nil {
				logger.Warnw("RabbitMQ connection closed", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}()

	// Create separate channels for each listener to avoid concurrency issues
	trustChannel, err := mqConn.Channel()
	if err != nil {
		logger.Fatalw("failed to create TRUST channel", "error", err)
	}
	defer trustChannel.Close()

	tdChannel, err := mqConn.Channel()
	if err != nil {
		logger.Fatalw("failed to create TD channel", "error", err)
	}
	defer tdChannel.Close()

	vstpChannel, err := mqConn.Channel()
	if err != nil {
		logger.Fatalw("failed to create VSTP channel", "error", err)
	}
	defer vstpChannel.Close()

	stompConn, err := utils.NewNRStompConnection()
	if err != nil {
		logger.Fatalw("failed to connect to NR stomp", "error", err)
	}

	var wg sync.WaitGroup

	trustListener := listener.NewListener(ctx, &wg, trustChannel, stompConn, "TRAIN_MVT_ALL_TOC", HandleTrust)
	trustListener.DeclareQueue("trust")

	tdListener := listener.NewListener(ctx, &wg, tdChannel, stompConn, "TD_ALL_SIG_AREA", HandleTD)
	tdListener.DeclareQueue("tdc")
	tdListener.DeclareQueue("tds")

	vstpListener := listener.NewListener(ctx, &wg, vstpChannel, stompConn, "VSTP_ALL", HandleVSTP)
	vstpListener.DeclareQueue("vstp")

	wg.Add(1)
	go trustListener.Start()

	wg.Add(1)
	go tdListener.Start()

	wg.Add(1)
	go vstpListener.Start()

	<-ctx.Done()
	stop()

	wg.Wait()

	stompConn.Disconnect()
}
