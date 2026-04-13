package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/heitormbonfim/asynq-mailhog-demo/internal/task"
)

type EmailHandler struct {
	client *asynq.Client
}

func NewEmailHandler(client *asynq.Client) *EmailHandler {
	return &EmailHandler{client: client}
}

type sendRequest struct {
	To      string `json:"to" binding:"required,email"`
	Subject string `json:"subject" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

type scheduleRequest struct {
	sendRequest
	SendAt string `json:"send_at" binding:"required"` // RFC3339: "2026-04-15T09:00:00-03:00"
}

type bulkRequest struct {
	Emails []sendRequest `json:"emails" binding:"required,dive"`
}

// POST /api/emails/send
func (h *EmailHandler) Send(c *gin.Context) {
	var req sendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := h.enqueue(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "email queued",
		"task_id": info.ID,
		"queue":   info.Queue,
	})
}

// POST /api/emails/schedule
func (h *EmailHandler) Schedule(c *gin.Context) {
	var req scheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sendAt, err := time.Parse(time.RFC3339, req.SendAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send_at must be RFC3339 format"})
		return
	}

	t, err := task.NewSendEmailTask(task.EmailPayload{
		To:      req.To,
		Subject: req.Subject,
		Body:    req.Body,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	info, err := h.client.Enqueue(t, asynq.ProcessAt(sendAt), asynq.MaxRetry(3))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":      "email scheduled",
		"task_id":      info.ID,
		"scheduled_at": sendAt.Format(time.RFC3339),
	})
}

// POST /api/emails/bulk
func (h *EmailHandler) Bulk(c *gin.Context) {
	var req bulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var results []gin.H
	for _, email := range req.Emails {
		info, err := h.enqueue(email)
		if err != nil {
			results = append(results, gin.H{"to": email.To, "error": "failed to enqueue"})
			continue
		}
		results = append(results, gin.H{"to": email.To, "task_id": info.ID})
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "bulk emails queued",
		"results": results,
	})
}

func (h *EmailHandler) enqueue(req sendRequest) (*asynq.TaskInfo, error) {
	t, err := task.NewSendEmailTask(task.EmailPayload{
		To:      req.To,
		Subject: req.Subject,
		Body:    req.Body,
	})
	if err != nil {
		return nil, err
	}
	return h.client.Enqueue(t, asynq.MaxRetry(3))
}
