package api

import (
	"log"

	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type APIServer struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}

func NewServer() (*APIServer, error) {
	db, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
		return nil, err
	}

	redis := utils.NewRedisClient()

	return &APIServer{
		DB:    db,
		Redis: redis,
	}, nil
}
