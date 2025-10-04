package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/go-stomp/stomp/v3"
	amqp "github.com/rabbitmq/amqp091-go"
)

type MsgType string

const (
	TrainActivation    MsgType = "0001"
	TrainCancellation  MsgType = "0002"
	TrainMovement      MsgType = "0003"
	UnidentifiedTrain  MsgType = "0004"
	TrainReinstatement MsgType = "0005"
	ChangeOfOrigin     MsgType = "0006"
	ChangeOfIdentity   MsgType = "0007"
	ChangeOfLocation   MsgType = "0008"
)

type TrustMessage struct {
	Header TrustHeader `json:"header"`
	Body   TrustBody   `json:"body"`
}

type TrustHeader struct {
	MsgType            MsgType `json:"msg_type"`
	MsgQueueTimestamp  string  `json:"msg_queue_timestamp"`
	SourceSystemID     string  `json:"source_system_id"`
	OriginalDataSource string  `json:"original_data_source"`
}

type TrustBody struct {
	TrainID              string `json:"train_id"`
	ActualTimestamp      string `json:"actual_timestamp"`
	LocStanox            string `json:"loc_stanox"`
	GBTTTimestamp        string `json:"gbtt_timestamp"`
	PlannedTimestamp     string `json:"planned_timestamp"`
	PlannedEventType     string `json:"planned_event_type"`
	EventType            string `json:"event_type"`
	EventSource          string `json:"event_source"`
	CorrectionInd        string `json:"correction_ind"`
	OffrouteInd          string `json:"offroute_ind"`
	DirectionInd         string `json:"direction_ind"`
	LineInd              string `json:"line_ind"`
	Platform             string `json:"platform"`
	Route                string `json:"route"`
	TrainServiceCode     string `json:"train_service_code"`
	DivisionCode         string `json:"division_code"`
	TOCID                string `json:"toc_id"`
	TimetableVariation   string `json:"timetable_variation"`
	VariationStatus      string `json:"variation_status"`
	NextReportStanox     string `json:"next_report_stanox"`
	NextReportRunTime    string `json:"next_report_run_time"`
	TrainTerminated      string `json:"train_terminated"`
	DelayMonitoringPoint string `json:"delay_monitoring_point"`
	ReportingStanox      string `json:"reporting_stanox"`
	AutoExpected         string `json:"auto_expected"`
}

func UnmarshalTrustMessages(data string) ([]TrustMessage, error) {
	var messages []TrustMessage
	err := json.Unmarshal([]byte(data), &messages)
	return messages, err
}

func main() {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	connection, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort))
	if err != nil {
		log.Fatal(err)
	}
	channel, err := connection.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()
	defer channel.Close()

	url := os.Getenv("NR_FEEDS_ENDPOINT")
	username := os.Getenv("NR_FEEDS_USERNAME")
	password := os.Getenv("NR_FEEDS_PASSWORD")

	conn, err := stomp.Dial("tcp", url,
		stomp.ConnOpt.Login(username, password),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Disconnect()

	topic := "/topic/TRAIN_MVT_ALL_TOC"
	sub, err := conn.Subscribe(topic, stomp.AckAuto)
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

		messages, err := UnmarshalTrustMessages(string(msg.Body))
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
