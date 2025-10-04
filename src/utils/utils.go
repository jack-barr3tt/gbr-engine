package utils

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-stomp/stomp/v3"
	"github.com/jack-barr3tt/gbr-engine/src/types"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewRabbitConnection() (*amqp.Connection, *amqp.Channel, error) {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	connection, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort))
	if err != nil {
		return nil, nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		return nil, nil, err
	}

	return connection, channel, nil
}

func NewNRStompConnection() (*stomp.Conn, error) {
	url := os.Getenv("NR_FEEDS_ENDPOINT")
	username := os.Getenv("NR_FEEDS_USERNAME")
	password := os.Getenv("NR_FEEDS_PASSWORD")

	conn, err := stomp.Dial("tcp", url,
		stomp.ConnOpt.Login(username, password),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func UnmarshalTrustMessages(data string) ([]types.TrustMessage, error) {
	var messages []types.TrustMessage
	err := json.Unmarshal([]byte(data), &messages)
	return messages, err
}
