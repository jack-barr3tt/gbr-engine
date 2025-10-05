package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
)

func main() {
	ctx := context.Background()

	rdb := utils.NewRedisClient()
	defer rdb.Close()

	conn, channel, err := utils.NewRabbitConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	defer channel.Close()

	_, err = channel.QueueDeclare("trust", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := channel.Consume("trust", "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Tracking train positions via TRUST feed...")

	for msg := range msgs {
		var trust types.TrustMessage
		if err := json.Unmarshal(msg.Body, &trust); err != nil {
			log.Printf("Bad JSON: %v", err)
			continue
		}

		if trust.Body.EventType == "ARRIVAL" || trust.Body.EventType == "DEPARTURE" {
			key := fmt.Sprintf("train:%s", trust.Body.TrainID)

			data := map[string]interface{}{
				"train_id":        trust.Body.TrainID,
				"event_type":      trust.Body.EventType,
				"location_stanox": trust.Body.LocStanox,
				"timestamp":       trust.Body.ActualTimestamp,
			}

			if err := rdb.HSet(ctx, key, data).Err(); err != nil {
				log.Printf("Redis HSet error: %v", err)
				continue
			}

			// optional TTL so Redis self-cleans stale trains
			rdb.Expire(ctx, key, 2*time.Hour)

			fmt.Printf("Updated Redis: %s %s @ %s\n",
				trust.Body.TrainID, trust.Body.EventType, trust.Body.LocStanox)
		}
	}
}
