package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

// Task Types & Payloads

const TypeSendEmail = "email:send"

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func NewSendEmailTask(payload EmailPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSendEmail, data), nil
}

// Email Handler (Worker)

func handleSendEmail(ctx context.Context, t *asynq.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[WORKER] Processing email task: %s -> %s", payload.Subject, payload.To)

	if err := sendEmailViaSMTP(payload); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("[SUCCESS] Email sent to %s", payload.To)
	return nil
}

func sendEmailViaSMTP(payload EmailPayload) error {
	smtpAddr := os.Getenv("SMTP_ADDR")
	if smtpAddr == "" {
		smtpAddr = "localhost:1025"
	}

	from := "noreply@example.com"
	message := fmt.Sprintf(
		"To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s",
		payload.To, from, payload.Subject, payload.Body,
	)

	return smtp.SendMail(smtpAddr, nil, from, []string{payload.To}, []byte(message))
}

// Worker mode: process tasks from Redis

func runWorker() {
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
	mux.HandleFunc(TypeSendEmail, handleSendEmail)

	log.Printf("Worker listening on Redis: %s", redisAddr)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("could not run server: %v", err)
	}
}

// Enqueuer mode: queue demo tasks

func runEnqueuer() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	defer client.Close()

	log.Printf("Connected to Redis: %s", redisAddr)

	// Immediate email
	enqueue(client, EmailPayload{
		To:      "user@example.com",
		Subject: "Welcome!",
		Body:    "Hello!\n\nYour account has been created.\n\nBest regards.",
	})

	// Delayed email (5 seconds)
	enqueue(client, EmailPayload{
		To:      "user@example.com",
		Subject: "Reminder: Deadline Tomorrow",
		Body:    "This is a reminder that your deadline is tomorrow at 10:00 AM.",
	}, asynq.ProcessIn(5*time.Second))

	// High priority email (via critical queue)
	enqueue(client, EmailPayload{
		To:      "admin@example.com",
		Subject: "URGENT: Review Required",
		Body:    "Urgent matter requires your immediate attention.",
	}, asynq.Queue("critical"))

	// Bulk enqueue
	for i := 1; i <= 5; i++ {
		enqueue(client, EmailPayload{
			To:      fmt.Sprintf("client%d@example.com", i),
			Subject: fmt.Sprintf("Update #%d", i),
			Body:    fmt.Sprintf("Your item %d has been updated.", i),
		})
	}

	log.Println("All tasks enqueued. Check MailHog at http://localhost:8025")
}

func enqueue(client *asynq.Client, payload EmailPayload, opts ...asynq.Option) {
	task, err := NewSendEmailTask(payload)
	if err != nil {
		log.Printf("Failed to create task for %s: %v", payload.To, err)
		return
	}
	opts = append(opts, asynq.MaxRetry(3))
	info, err := client.Enqueue(task, opts...)
	if err != nil {
		log.Printf("Failed to enqueue task for %s: %v", payload.To, err)
		return
	}
	log.Printf("Enqueued: %s (ID: %s, Queue: %s)", payload.To, info.ID, info.Queue)
}

func main() {
	mode := "enqueuer"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	log.Println(strings.Repeat("=", 50))
	log.Printf("Asynq + MailHog Demo - Mode: %s", mode)
	log.Println(strings.Repeat("=", 50))

	switch mode {
	case "worker":
		runWorker()
	case "enqueuer":
		runEnqueuer()
	default:
		fmt.Println("Usage: go run main.go [worker|enqueuer]")
		os.Exit(1)
	}
}
