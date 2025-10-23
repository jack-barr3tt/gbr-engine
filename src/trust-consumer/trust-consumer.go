package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jack-barr3tt/gbr-engine/src/common/types"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	utils.InitLogger()
	defer utils.SyncLogger()
	logger := utils.GetLogger()
	ctx := context.Background()

	db, err := utils.NewPostgresConnection()
	if err != nil {
		logger.Fatalw("failed to connect to Postgres", "error", err)
	}
	defer db.Close()

	rdb := utils.NewRedisClient()
	defer rdb.Close()

	conn, channel, err := utils.NewRabbitConnection()
	if err != nil {
		logger.Fatalw("failed to connect to RabbitMQ", "error", err)
	}
	defer conn.Close()
	defer channel.Close()

	_, err = channel.QueueDeclare("trust", false, false, false, false, nil)
	if err != nil {
		logger.Fatalw("failed to declare TRUST queue", "error", err)
	}

	msgs, err := channel.Consume("trust", "", true, false, false, false, nil)
	if err != nil {
		logger.Fatalw("failed to consume TRUST queue", "error", err)
	}
	logger.Infow("tracking train positions via TRUST feed")

	for msg := range msgs {
		var trust types.TrustMessage
		if err := json.Unmarshal(msg.Body, &trust); err != nil {
			logger.Warnw("bad json in TRUST message", "error", err)
			continue
		}

		switch trust.Header.MsgType {
		case types.TrainActivation:
			if err := processActivation(ctx, rdb, logger, &trust.Body); err != nil {
				logger.Warnw("error processing activation", "train_id", trust.Body.TrainID, "error", err)
			}
		case types.TrainMovement:
			if err := processMovement(ctx, db, rdb, logger, &trust.Body); err != nil {
				logger.Warnw("error processing trust event", "train_id", trust.Body.TrainID, "error", err)
			}
		default:
			continue
		}
	}
}

func processActivation(ctx context.Context, rdb *redis.Client, logger *zap.SugaredLogger, trust *types.TrustBody) error {
	trainID := strings.TrimSpace(trust.TrainID)
	trainUID := strings.TrimSpace(trust.TrainUID)

	key := utils.BuildActivationKey(trainID)
	err := rdb.Set(ctx, key, trainUID, 48*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to store activation: %w", err)
	}
	logger.Infow("stored activation", "train_id", trainID, "train_uid", trainUID)
	return nil
}

func processMovement(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client, logger *zap.SugaredLogger, trust *types.TrustBody) error {
	runDate := utils.FormatRunDate(time.Now())
	trainID := strings.TrimSpace(trust.TrainID)

	activationKey := utils.BuildActivationKey(trainID)
	trainUID, err := rdb.Get(ctx, activationKey).Result()
	if err != nil {
		logger.Debugw("no activation found for train", "train_id", trainID)
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
		logger.Debugw("no stanox match in schedule",
			"train_uid", trainUID,
			"loc_stanox", trust.LocStanox,
			"schedule_stanoxes", foundStanoxes,
		)
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

	logger.Infow("merged TRUST into schedule",
		"train_uid", trainUID,
		"train_id", trainID,
		"event_type", trust.EventType,
		"stanox", trust.LocStanox,
	)

	return nil
}
