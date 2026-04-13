# Asynq Email Queue Example

A Go tutorial project that demonstrates background job processing using [Asynq](https://github.com/hibiken/asynq) (a Redis-based task queue for Go), [Gin](https://github.com/gin-gonic/gin) (HTTP framework), and [MailHog](https://github.com/mailhog/MailHog) (a fake SMTP server for testing). The API accepts email requests over HTTP, pushes them as tasks into Redis, and a separate worker process picks them up and delivers them via SMTP.

No cron jobs are involved. Asynq is an **event-driven task queue**, not a scheduler. Tasks are processed as soon as they become available (or after a specified delay), not on a fixed time interval.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [How Asynq Works Under the Hood](#how-asynq-works-under-the-hood)
  - [Task Lifecycle](#task-lifecycle)
  - [Queue Priority (Weighted Random)](#queue-priority-weighted-random)
  - [Scheduling and Delays](#scheduling-and-delays)
  - [Retries on Failure](#retries-on-failure)
  - [Concurrency](#concurrency)
- [How This Project Is Structured](#how-this-project-is-structured)
  - [The API (cmd/api)](#the-api-cmdapi)
  - [The Worker (cmd/worker)](#the-worker-cmdworker)
  - [Task Definitions (internal/task)](#task-definitions-internaltask)
  - [HTTP Handlers (internal/handler)](#http-handlers-internalhandler)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
  - [Playground](#playground)
- [API Endpoints](#api-endpoints)
- [Project Structure](#project-structure)
- [Environment Variables](#environment-variables)
- [Ports](#ports)

## Architecture Overview

```
                    +-----------+
  HTTP request ---->|  API      |
                    |  (Gin)    |
                    +-----+-----+
                          |
                    enqueue task
                    (JSON payload)
                          |
                          v
                    +-----+-----+
                    |   Redis   |
                    |  (queue)  |
                    +-----+-----+
                          |
                    deliver task
                    (when ready)
                          |
                          v
                    +-----+-----+
                    |  Worker   |
                    |  (Asynq)  |
                    +-----+-----+
                          |
                    send email
                    (net/smtp)
                          |
                          v
                    +-----+-----+
                    |  MailHog  |
                    |  (SMTP)   |
                    +-----------+
```

The API and the worker are **separate OS processes**. They never call each other directly. Redis is the only thing connecting them. This means:

- You can restart the API without affecting in-progress email deliveries.
- You can run multiple worker instances to process tasks faster.
- If the worker is down, tasks accumulate in Redis and get processed when it comes back up.

## How Asynq Works Under the Hood

### Task Lifecycle

When you enqueue a task, here is exactly what happens step by step:

1. **Client creates a task** - Your code calls `asynq.NewTask(typeName, jsonPayload)`, which builds a task object with a type string (like `"email:send"`) and a `[]byte` payload (the JSON-serialized email data).

2. **Client pushes to Redis** - Calling `client.Enqueue(task)` writes the task into a Redis list. Each queue (e.g. `"default"`, `"critical"`) is a separate Redis list. The task gets a unique ID and is stored along with metadata (retry count, queue name, creation time).

3. **Worker polls Redis** - The worker runs a continuous loop internally. It uses Redis `BRPOP` (blocking pop) to pull the next task from the queue. This is not polling on a timer — `BRPOP` blocks until a task is available, so the worker reacts instantly when a new task arrives with zero delay.

4. **Worker matches the task type to a handler** - The worker's `ServeMux` maps task types to handler functions. When it pulls a task with type `"email:send"`, it calls the registered `HandleSendEmail` function.

5. **Handler runs** - The handler deserializes the JSON payload and does the actual work (in our case, sending an email via SMTP to MailHog).

6. **Success or retry** - If the handler returns `nil`, the task is marked as completed and removed from Redis. If it returns an error, Asynq puts the task back into a retry queue and will attempt it again later (with exponential backoff), up to `MaxRetry` times.

### Queue Priority (Weighted Random)

```go
Queues: map[string]int{
    "critical": 6,
    "default":  3,
    "low":      1,
}
```

These numbers are **relative weights**, not absolute counts or intervals. When the worker is ready to pick up the next task, Asynq uses **weighted random selection** to decide which queue to pull from. Here is how it works:

- The total weight is `6 + 3 + 1 = 10`.
- Each time the worker needs a task, it picks a random number from 1 to 10.
  - Numbers 1-6 (60% chance) -> pull from `"critical"`
  - Numbers 7-9 (30% chance) -> pull from `"default"`
  - Number 10  (10% chance) -> pull from `"low"`

This means that **on average**, the worker picks from the `"critical"` queue 6 out of every 10 times. It does not mean "process 6 critical tasks, then 3 default, then 1 low" in a fixed rotation. It is probabilistic, so over a large number of tasks the distribution converges to those ratios.

If a higher-priority queue is empty, the worker simply picks from the next available queue. No tasks are ever starved — even `"low"` tasks get processed, just less frequently when higher-priority queues have work.

### Scheduling and Delays

Asynq supports delaying task execution. This is **not a cron job**. There is no recurring schedule. It is a one-shot delay: the task fires once after the specified time.

| Option | What happens in Redis | Example |
|--------|----------------------|---------|
| *(none)* | Task goes directly into the queue's ready list. Worker picks it up immediately. | `client.Enqueue(task)` |
| `asynq.ProcessIn(dur)` | Task goes into a **scheduled set** (a Redis sorted set keyed by timestamp). Asynq's internal scheduler checks this set every second, and moves tasks to the ready list when their time arrives. | `client.Enqueue(task, asynq.ProcessIn(5*time.Minute))` |
| `asynq.ProcessAt(time)` | Same mechanism as `ProcessIn`, but you provide an absolute `time.Time` instead of a duration. | `client.Enqueue(task, asynq.ProcessAt(tomorrow9AM))` |

Under the hood, the Asynq server runs a background goroutine called the **scheduler** (not to be confused with cron). Every second, it runs `ZRANGEBYSCORE` on the scheduled set to find tasks whose execution time has passed, then moves them to the ready list with `RPUSH`. This is why there can be up to ~1 second of delay after the scheduled time before a task actually starts processing.

### Retries on Failure

When a task handler returns an error:

1. The task's retry counter increments.
2. If retries remain (`< MaxRetry`), the task moves to a retry queue with **exponential backoff** (the delay between retries increases: ~15s, ~30s, ~1min, etc.).
3. If all retries are exhausted, the task moves to the **dead queue** (also called the archive). Dead tasks stay there for inspection but are not retried automatically.

In this project, every task is enqueued with `asynq.MaxRetry(3)`, so a failing email gets 3 additional attempts before it is archived.

### Concurrency

```go
asynq.Config{
    Concurrency: 5,
}
```

This means the worker runs **5 goroutines** in parallel, each pulling and processing tasks independently. If you enqueue 20 emails at once, 5 will be processed simultaneously, and the remaining 15 wait in Redis until a goroutine becomes free. Increase this number to handle more tasks in parallel (at the cost of more memory, CPU, and SMTP connections).

## How This Project Is Structured

### The API (cmd/api)

`cmd/api/main.go` starts a Gin HTTP server and creates an Asynq **client**. The client is the only thing the API uses from Asynq — it connects to Redis and provides the `Enqueue()` method. The API itself never processes tasks; it just pushes them.

```go
client := asynq.NewClient(asynq.RedisClientOpt{Addr: "localhost:6379"})
// later...
client.Enqueue(task, asynq.MaxRetry(3))
```

The API registers three routes under `/api/emails` and passes the client to the handler.

### The Worker (cmd/worker)

`cmd/worker/main.go` starts an Asynq **server** (the worker). It creates a `ServeMux` that maps task types to handler functions, exactly like an HTTP mux maps routes to handlers:

```go
mux := asynq.NewServeMux()
mux.HandleFunc("email:send", task.HandleSendEmail)
srv.Run(mux)
```

`srv.Run(mux)` blocks forever, pulling tasks from Redis and dispatching them to the right handler. It also runs background goroutines for the scheduler (delayed tasks) and the retry manager.

### Task Definitions (internal/task)

`internal/task/email.go` defines:
- `TypeSendEmail = "email:send"` — the task type string that both the API and worker use to identify this kind of task.
- `EmailPayload` — the struct that gets serialized to JSON and attached to the task.
- `NewSendEmailTask()` — a helper that creates an `asynq.Task` from a payload.
- `HandleSendEmail()` — the worker-side function that deserializes the payload and calls `sendEmail()`.
- `sendEmail()` — the actual SMTP delivery using Go's `net/smtp` package.

### HTTP Handlers (internal/handler)

`internal/handler/email.go` contains the Gin handlers for each endpoint:
- `Send()` — validates the request body, creates a task, enqueues it immediately.
- `Schedule()` — same as Send but parses the `send_at` field and passes `asynq.ProcessAt(sendAt)` to delay the task.
- `Bulk()` — loops over an array of emails and enqueues each one as a separate task.

All three return HTTP `202 Accepted` with the task ID from Redis, so the caller knows the task was queued (not necessarily sent yet).

## Prerequisites

- Go 1.21+
- Podman & Podman Compose (or Docker & Docker Compose)

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

Check emails at http://localhost:8025 (MailHog Web UI).

**What happens when you run `make send`:**
1. The shell script sends a POST request to `localhost:8080/api/emails/send`.
2. The API handler validates the JSON body, creates an `asynq.Task` with type `"email:send"` and the email data as a JSON payload, and calls `client.Enqueue()`.
3. The task gets written to the `"default"` queue in Redis.
4. The worker (which is blocked on `BRPOP`) immediately wakes up, pulls the task, and calls `HandleSendEmail`.
5. `HandleSendEmail` unmarshals the JSON payload and calls Go's `net/smtp.SendMail()` to deliver the email to MailHog on port 1025.
6. MailHog catches the email and displays it in its web UI on port 8025.

### Playground

The playground is a self-contained single-file demo at `cmd/playground/main.go`. It bundles both a worker and an enqueuer in one file — no HTTP server, no separate packages. This is useful for quickly experimenting with Asynq features without running the full API.

```bash
# 1. Start Redis + MailHog (if not already running)
make up

# 2. Start the playground worker (terminal 1)
go run ./cmd/playground worker

# 3. Enqueue demo tasks (terminal 2)
go run ./cmd/playground enqueuer
```

**What the enqueuer does:**
- Sends 1 email immediately (goes straight to the ready list).
- Sends 1 email with a 5-second delay using `asynq.ProcessIn(5 * time.Second)` (goes to the scheduled set, moved to ready after 5s).
- Sends 1 email to the `"critical"` queue using `asynq.Queue("critical")` (gets picked up with 60% probability over default/low).
- Sends 5 emails in a loop to simulate a bulk operation.

You can edit `cmd/playground/main.go` directly to experiment. Try changing:
- `asynq.ProcessIn(30 * time.Second)` — delay a task by 30 seconds and watch it appear in MailHog later.
- `asynq.Queue("low")` — put a task in the low-priority queue and see it get picked up less often when other queues have work.
- `asynq.MaxRetry(0)` — disable retries and see what happens when you stop MailHog mid-run (`make down` then `make up` to restart).
- `Concurrency: 1` in `runWorker()` — force serial processing and watch tasks get handled one by one.

## API Endpoints

### POST /api/emails/send

Send an email immediately. The task goes into the `"default"` queue and the worker picks it up as soon as possible.

```bash
curl -X POST http://localhost:8080/api/emails/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user@example.com", "subject": "Hello", "body": "Test email"}'
```

**Response** (HTTP 202):
```json
{
  "message": "email queued",
  "task_id": "abc123",
  "queue": "default"
}
```

### POST /api/emails/schedule

Schedule an email for a specific date and time. The `send_at` field uses [RFC 3339](https://datatracker.ietf.org/doc/html/rfc3339) format (e.g. `2026-04-15T09:00:00-03:00`). The task sits in Redis's scheduled set until that time arrives, then moves to the ready list for the worker to pick up.

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

**Response** (HTTP 202):
```json
{
  "message": "email scheduled",
  "task_id": "def456",
  "scheduled_at": "2026-04-15T09:00:00-03:00"
}
```

### POST /api/emails/bulk

Send emails to multiple recipients. Each email becomes a separate task in Redis, so they can be processed in parallel by the worker's goroutines.

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

**Response** (HTTP 202):
```json
{
  "message": "bulk emails queued",
  "results": [
    {"to": "alice@example.com", "task_id": "ghi789"},
    {"to": "bob@example.com",   "task_id": "jkl012"}
  ]
}
```

## Project Structure

```
├── cmd/
│   ├── api/main.go            # Gin HTTP server + Asynq client (enqueues tasks)
│   ├── playground/main.go     # Self-contained demo (worker + enqueuer in one file)
│   └── worker/main.go         # Asynq server (processes tasks from Redis)
├── internal/
│   ├── handler/email.go       # Gin HTTP handlers (request validation + enqueue)
│   └── task/email.go          # Task type, payload struct, handler, SMTP sender
├── scripts/
│   ├── send-email.sh          # curl: send an email immediately
│   ├── schedule-email.sh      # curl: schedule an email for later
│   └── send-bulk.sh           # curl: send emails to multiple recipients
├── .env.example               # Default environment variables
├── docker-compose.yml         # Redis + MailHog containers
├── Makefile                   # Shortcuts for all common commands
└── go.mod
```

## Environment Variables

Copy `.env.example` to `.env` and adjust as needed.

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_ADDR` | `localhost:6379` | Redis connection address (host:port). Both the API and worker need this. |
| `SMTP_ADDR` | `localhost:1025` | SMTP server address. The worker sends emails here. MailHog listens on 1025 by default. |
| `PORT` | `8080` | The port the Gin API server listens on. Only used by `cmd/api`. |

## Ports

| Service | Port | Purpose |
|---------|------|---------|
| API (Gin) | 8080 | Receives HTTP requests and enqueues tasks |
| Redis | 6379 | Stores task queues, scheduled sets, and retry data |
| MailHog SMTP | 1025 | Catches emails sent by the worker |
| MailHog Web UI | 8025 | Browser UI to view caught emails |
