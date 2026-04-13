#!/bin/bash
# Send an email immediately

curl -s -X POST http://localhost:8080/api/emails/send \
  -H "Content-Type: application/json" \
  -d '{
    "to": "user@example.com",
    "subject": "Welcome!",
    "body": "Hello! Your account has been created."
  }' | jq .
