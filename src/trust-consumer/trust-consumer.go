package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	db, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

		switch trust.Header.MsgType {
		case types.TrainActivation:
			if err := processActivation(ctx, rdb, &trust.Body); err != nil {
				log.Printf("Error processing activation for train %s: %v", trust.Body.TrainID, err)
			}
		case types.TrainMovement:
			if err := processMovement(ctx, db, rdb, &trust.Body); err != nil {
				log.Printf("Error processing TRUST event for train %s: %v", trust.Body.TrainID, err)
			}
		default:
			continue
		}
	}
}

func processActivation(ctx context.Context, rdb *redis.Client, trust *types.TrustBody) error {
	trainID := strings.TrimSpace(trust.TrainID)
	trainUID := strings.TrimSpace(trust.TrainUID)

	key := utils.BuildActivationKey(trainID)
	err := rdb.Set(ctx, key, trainUID, 48*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to store activation: %w", err)
	}

	fmt.Printf("Stored activation: %s → %s\n", trainID, trainUID)
	return nil
}

func processMovement(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, trust *types.TrustBody) error {
	runDate := utils.FormatRunDate(time.Now())
	trainID := strings.TrimSpace(trust.TrainID)

	activationKey := utils.BuildActivationKey(trainID)
	trainUID, err := rdb.Get(ctx, activationKey).Result()
	if err != nil {
		fmt.Printf("No activation found for train_id %s\n", trainID)
		return nil
	}

	trainUID = strings.TrimSpace(trainUID)

	journey, err := utils.LoadTrainJourney(ctx, db, rdb, trainUID, runDate)
	if err != nil {
		return nil
	}

	merged := utils.MergeTrustEvent(&journey, trust)
	if !merged {
		foundStanoxes := []string{}
		for _, stop := range journey.Stops {
			foundStanoxes = append(foundStanoxes, stop.Stanox)
		}
		fmt.Printf("No stanox match for %s (looking for %s, schedule has: %v)\n",
			trainUID, trust.LocStanox, foundStanoxes)
		return nil
	}

	b, err := json.Marshal(journey)
	if err != nil {
		return fmt.Errorf("failed to marshal journey: %w", err)
	}
	schedKey := utils.BuildScheduleKey(trainUID, runDate)
	if err := rdb.Set(ctx, schedKey, b, 48*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save merged schedule: %w", err)
	}

	fmt.Printf("Merged TRUST into schedule: %s (%s) %s @ %s\n",
		trainUID, trainID, trust.EventType, trust.LocStanox)

	return nil
}
