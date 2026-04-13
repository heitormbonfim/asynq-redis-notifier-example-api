package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"

	"github.com/hibiken/asynq"
)

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

func HandleSendEmail(ctx context.Context, t *asynq.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[WORKER] Processing: %s -> %s", payload.Subject, payload.To)

	if err := sendEmail(payload); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("[WORKER] Sent: %s -> %s", payload.Subject, payload.To)
	return nil
}

func sendEmail(payload EmailPayload) error {
	smtpAddr := os.Getenv("SMTP_ADDR")
	if smtpAddr == "" {
		smtpAddr = "localhost:1025"
	}

	from := "noreply@example.com"
	msg := fmt.Sprintf(
		"To: %s\r\nFrom: %s\r\nSubject: %s\r\n\r\n%s",
		payload.To, from, payload.Subject, payload.Body,
	)

	return smtp.SendMail(smtpAddr, nil, from, []string{payload.To}, []byte(msg))
}
