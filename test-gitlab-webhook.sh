#!/bin/bash

# Test script to send a simulated GitLab MR webhook to Telefonistka server
#
# Usage: ./test-gitlab-webhook.sh

WEBHOOK_URL="${WEBHOOK_URL:-http://localhost:8080/webhook}"

echo "🧪 Testing GitLab Webhook Integration"
echo "Target: $WEBHOOK_URL"
echo ""

# Create a test MR webhook payload
PAYLOAD=$(cat <<'EOF'
{
  "object_kind": "merge_request",
  "event_type": "merge_request",
  "user": {
    "id": 1,
    "name": "Administrator",
    "username": "root"
  },
  "project": {
    "id": 1,
    "name": "test",
    "path_with_namespace": "root/test",
    "web_url": "http://gitlab.localhost/root/test",
    "default_branch": "main"
  },
  "object_attributes": {
    "id": 1,
    "iid": 1,
    "title": "Test MR from Telefonistka",
    "state": "opened",
    "action": "open",
    "source_branch": "feature-test",
    "target_branch": "main",
    "last_commit": {
      "id": "abc123def456",
      "message": "Test commit"
    },
    "work_in_progress": false,
    "merge_status": "can_be_merged"
  }
}
EOF
)

echo "📤 Sending GitLab MR webhook..."
echo ""

RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" \
  -X POST \
  -H "Content-Type: application/json" \
  -H "X-Gitlab-Event: Merge Request Hook" \
  -H "X-Gitlab-Token: test-secret" \
  -d "$PAYLOAD" \
  "$WEBHOOK_URL")

HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
BODY=$(echo "$RESPONSE" | sed '/HTTP_CODE:/d')

echo "Response:"
echo "$BODY"
echo ""
echo "HTTP Status: $HTTP_CODE"
echo ""

if [ "$HTTP_CODE" == "200" ]; then
    echo "✅ Webhook accepted successfully!"
else
    echo "❌ Webhook failed with status $HTTP_CODE"
    exit 1
fi
