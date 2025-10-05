package utils

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-stomp/stomp/v3"
	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
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

func NewRabbitConnectionOnly() (*amqp.Connection, error) {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	connection, err := amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort))
	if err != nil {
		return nil, err
	}

	return connection, nil
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

func NewRedisClient() *redis.Client {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		// default to the redis service in the cluster
		redisAddr = "redis:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	return rdb
}

func UnmarshalTrustMessages(data string) ([]types.TrustMessage, error) {
	var messages []types.TrustMessage
	err := json.Unmarshal([]byte(data), &messages)
	return messages, err
}

func UnmarshalTDMessages(data string) ([]types.TDCMsgBody, []types.TDSMsgBody, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(data), &raws); err != nil {
		return nil, nil, err
	}

	var tdCMsgs []types.TDCMsgBody
	var tdSMsgs []types.TDSMsgBody

	for _, raw := range raws {
		var tdC types.TDCMsgEnvelope
		if err := json.Unmarshal(raw, &tdC); err == nil {
			if tdC.CAMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CAMsgBody)
			}
			if tdC.CBMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CBMsgBody)
			}
			if tdC.CCMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CCMsgBody)
			}
			if tdC.CTMsgBody != nil {
				tdCMsgs = append(tdCMsgs, *tdC.CTMsgBody)
			}
			continue
		}

		var tdS types.TDSMsgEnvelope
		if err := json.Unmarshal(raw, &tdS); err == nil {
			if tdS.SFMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SFMsgBody)
			}
			if tdS.SGMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SGMsgBody)
			}
			if tdS.SHMsgBody != nil {
				tdSMsgs = append(tdSMsgs, *tdS.SHMsgBody)
			}
			continue
		}
	}

	return tdCMsgs, tdSMsgs, nil
}
