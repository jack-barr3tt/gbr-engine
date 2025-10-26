package data

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type DataClient struct {
	pg     *pgxpool.Pool
	rdb    *redis.Client
	logger *zap.SugaredLogger
}

func NewDataClient(db *pgxpool.Pool, rdb *redis.Client, logger *zap.SugaredLogger) *DataClient {
	return &DataClient{
		pg:  db,
		rdb: rdb,
	}
}
