# Asynq Email Queue Example

A simple Go/Gin API that queues email tasks with [Asynq](https://github.com/hibiken/asynq) and sends them through [MailHog](https://github.com/mailhog/MailHog) for local testing.

## How Asynq Works

```
API (Gin)                  Redis                  Worker
   |                         |                       |
   |--- enqueue task ------->|                       |
   |                         |--- deliver task ----->|
   |                         |                       |--- handle task
   |                         |                       |--- send email via SMTP
   |                         |<--- done/retry -------|
```

1. **API** receives an HTTP request, creates a task (type + JSON payload), and pushes it to Redis.
2. **Redis** holds the task in a queue. If a delay or scheduled time is set, it waits in a scheduled set until then.
3. **Worker** pulls tasks from Redis, matches the type to a handler, and runs it. On failure, asynq retries automatically up to `MaxRetry` times.

The API and worker are separate processes — they only communicate through Redis. You can scale workers independently.

### Priority via queues

```go
Queues: map[string]int{
    "critical": 6,  // picked ~6x more often
    "default":  3,
    "low":      1,
}
```

Assign a task to a queue: `asynq.Queue("critical")`

### Scheduling

| Option | Effect |
|--------|--------|
| *(none)* | Process immediately |
| `asynq.ProcessIn(5 * time.Minute)` | Process after a delay |
| `asynq.ProcessAt(time.Time)` | Process at an exact date/time |

## Prerequisites

- Go 1.21+
- Podman & Podman Compose

## Quick Start

```bash
# 1. Start Redis + MailHog
make up

# 2. Start the worker (terminal 1)
make worker

# 3. Start the API (terminal 2)
make api

# 4. Test the endpoints (terminal 3)
make send        # send an email now
make schedule    # schedule an email for a specific date/time
make bulk        # send emails to multiple recipients
```

Check emails at http://localhost:8025

## API Endpoints

### POST /api/emails/send

Send an email immediately.

```bash
curl -X POST http://localhost:8080/api/emails/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user@example.com", "subject": "Hello", "body": "Test email"}'
```

### POST /api/emails/schedule

Schedule an email for a specific date and time. `send_at` uses RFC3339 format.

```bash
curl -X POST http://localhost:8080/api/emails/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Reminder",
    "body": "Your meeting is tomorrow.",
    "send_at": "2026-04-15T09:00:00-03:00"
  }'
```

The task sits in Redis until that time arrives, then the worker picks it up and sends the email.

### POST /api/emails/bulk

Send emails to multiple recipients at once.

```bash
curl -X POST http://localhost:8080/api/emails/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "emails": [
      {"to": "alice@example.com", "subject": "Update", "body": "Item updated."},
      {"to": "bob@example.com",   "subject": "Update", "body": "Item updated."}
    ]
  }'
```

## Project Structure

```
├── cmd/
│   ├── api/main.go            # Gin HTTP server + asynq client
│   └── worker/main.go         # Asynq worker server
├── internal/
│   ├── handler/email.go       # HTTP handlers (send, schedule, bulk)
│   └── task/email.go          # Task types, payload, worker handler, SMTP
├── scripts/
│   ├── send-email.sh          # Test: send immediately
│   ├── schedule-email.sh      # Test: schedule for later
│   └── send-bulk.sh           # Test: bulk send
├── docker-compose.yml         # Redis + MailHog
├── Makefile
└── go.mod
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `SMTP_ADDR` | `localhost:1025` | SMTP server address |
| `PORT` | `8080` | API server port |

## Ports

| Service | Port | Purpose |
|---------|------|---------|
| API | 8080 | HTTP endpoints |
| Redis | 6379 | Task queue |
| MailHog SMTP | 1025 | Email ingestion |
| MailHog Web UI | 8025 | View sent emails |
