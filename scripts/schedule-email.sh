#!/bin/bash
# Schedule an email for a specific date and time

curl -s -X POST http://localhost:8080/api/emails/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Reminder: Meeting Tomorrow",
    "body": "Do not forget your meeting at 10:00 AM.",
    "send_at": "2026-04-15T09:00:00-03:00"
  }' | jq .
