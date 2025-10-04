package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jack-barr3tt/gbr-engine/src/queuer/listener"
	"github.com/jack-barr3tt/gbr-engine/src/utils"

	amqp "github.com/rabbitmq/amqp091-go"
)

func HandleTrust(channel *amqp.Channel, data string) {
	messages, err := utils.UnmarshalTrustMessages(data)
	if err != nil {
		log.Println("Error unmarshalling message:", err)
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
			log.Println("Error publishing message to RabbitMQ:", err)
		} else {
			fmt.Println("Published message to RabbitMQ for TRUST")
		}
	}
}

func HandleTD(channel *amqp.Channel, data string) {
	tdcMessages, tdsMessages, err := utils.UnmarshalTDMessages(data)
	if err != nil {
		log.Println("Error unmarshalling message:", err)
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
			log.Println("Error publishing message to RabbitMQ:", err)
		} else {
			fmt.Println("Published message to RabbitMQ for TD-C")
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
			log.Println("Error publishing message to RabbitMQ:", err)
		} else {
			fmt.Println("Published message to RabbitMQ for TD-S")
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mqConn, channel, err := utils.NewRabbitConnection()
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ:", err)
	}
	defer mqConn.Close()
	defer channel.Close()

	stompConn, err := utils.NewNRStompConnection()
	if err != nil {
		log.Fatal("Failed to connect to NR Stomp:", err)
	}

	var wg sync.WaitGroup

	trustListener := listener.NewListener(ctx, &wg, channel, stompConn, "TRAIN_MVT_ALL_TOC", HandleTrust)
	tdListener := listener.NewListener(ctx, &wg, channel, stompConn, "TD_ALL_SIG_AREA", HandleTD)

	wg.Add(1)
	go trustListener.Start()

	wg.Add(1)
	go tdListener.Start()

	<-ctx.Done()
	stop()

	wg.Wait()

	stompConn.Disconnect()
}
