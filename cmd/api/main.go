package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/heitormbonfim/asynq-mailhog-demo/internal/handler"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	h := handler.NewEmailHandler(client)

	r := gin.Default()

	api := r.Group("/api/emails")
	{
		api.POST("/send", h.Send)
		api.POST("/schedule", h.Schedule)
		api.POST("/bulk", h.Bulk)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("API server starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
