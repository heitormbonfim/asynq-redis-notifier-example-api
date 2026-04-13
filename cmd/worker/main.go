package main

import (
	"log"
	"os"

	"github.com/hibiken/asynq"

	"github.com/heitormbonfim/asynq-mailhog-demo/internal/task"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 5,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(task.TypeSendEmail, task.HandleSendEmail)

	log.Printf("Worker starting on Redis: %s", redisAddr)
	if err := srv.Run(mux); err != nil {
		log.Fatal(err)
	}
}
