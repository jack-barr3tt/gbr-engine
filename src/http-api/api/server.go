package api

import (
	"log"

	"github.com/jack-barr3tt/gbr-engine/src/common/utils"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APIServer struct {
	DB *pgxpool.Pool
}

func NewServer() (*APIServer, error) {
	db, err := utils.NewPostgresConnection()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
		return nil, err
	}

	return &APIServer{
		DB: db,
	}, nil
}
