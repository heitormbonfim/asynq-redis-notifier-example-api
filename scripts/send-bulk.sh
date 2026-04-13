#!/bin/bash
# Send emails to multiple recipients at once

curl -s -X POST http://localhost:8080/api/emails/bulk \
  -H "Content-Type: application/json" \
  -d '{
    "emails": [
      {"to": "alice@example.com", "subject": "Update #1", "body": "Your item has been updated."},
      {"to": "bob@example.com",   "subject": "Update #2", "body": "Your item has been updated."},
      {"to": "carol@example.com", "subject": "Update #3", "body": "Your item has been updated."}
    ]
  }' | jq .
