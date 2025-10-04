package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/jack-barr3tt/gbr-engine/src/utils"

	"github.com/go-stomp/stomp/v3"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
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
	defer stompConn.Disconnect()

	topic := "/topic/TRAIN_MVT_ALL_TOC"
	sub, err := stompConn.Subscribe(topic, stomp.AckAuto)
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()

	fmt.Println("Listening for messages on topic:", topic)

	for {
		msg := <-sub.C
		if msg.Err != nil {
			log.Println("Error receiving message:", msg.Err)
			continue
		}

		messages, err := utils.UnmarshalTrustMessages(string(msg.Body))
		if err != nil {
			log.Println("Error unmarshalling message:", err)
			continue
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
			}

			fmt.Printf("Published %[1]s message for train %[2]s to RabbitMQ\n", message.Header.MsgType, message.Body.TrainID)
		}
	}
}
