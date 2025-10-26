package utils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-stomp/stomp/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

func NewRabbitConnection() (*amqp.Connection, *amqp.Channel, error) {
	mqUser := os.Getenv("MQ_USER")
	mqPassword := os.Getenv("MQ_PASSWORD")
	mqHost := os.Getenv("MQ_HOST")
	mqPort := os.Getenv("MQ_PORT")

	config := amqp.Config{
		Heartbeat: 60 * time.Second,
		Locale:    "en_US",
	}

	connection, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort), config)
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

	config := amqp.Config{
		Heartbeat: 60 * time.Second,
		Locale:    "en_US",
	}

	connection, err := amqp.DialConfig(fmt.Sprintf("amqp://%s:%s@%s:%s/", mqUser, mqPassword, mqHost, mqPort), config)
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

func NewPostgresConnection() (*pgxpool.Pool, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")

	dbConnectionString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	connection, err := pgxpool.New(context.Background(), dbConnectionString)
	if err != nil {
		return nil, err
	}

	return connection, nil
}
