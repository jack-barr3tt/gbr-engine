package api

import (
	"github.com/jack-barr3tt/gbr-engine/src/common/data"
	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type APIServer struct {
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Logger *zap.SugaredLogger
	Data   *data.DataClient
}

func NewServer() (*APIServer, error) {
	db, err := utils.NewPostgresConnection()
	logger := utils.GetLogger()
	if err != nil {
		logger.Errorw("failed to connect to database", "error", err)
		return nil, err
	}

	redis := utils.NewRedisClient()

	data := data.NewDataClient(db, redis, logger)

	return &APIServer{
		DB:     db,
		Redis:  redis,
		Logger: logger,
		Data:   data,
	}, nil
}
