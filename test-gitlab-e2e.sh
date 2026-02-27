#!/bin/bash

# End-to-end test: Create a real MR in GitLab and test webhook delivery
#
# This script:
# 1. Creates a test branch in GitLab
# 2. Makes a commit
# 3. Creates an MR
# 4. Verifies Telefonistka responds

set -e

GITLAB_URL="${GITLAB_URL:-http://gitlab.localhost}"
GITLAB_TOKEN="${GITLAB_TOKEN:?GITLAB_TOKEN is required}"
PROJECT="root/test"

echo "🧪 End-to-End GitLab Webhook Test"
echo "================================="
echo ""
echo "GitLab URL: $GITLAB_URL"
echo "Project: $PROJECT"
echo ""

# URL-encode the project path
PROJECT_ENCODED=$(echo -n "$PROJECT" | jq -sRr @uri)

# API base
API_BASE="$GITLAB_URL/api/v4"

echo "📋 Step 1: Get project info..."
PROJECT_INFO=$(curl -s \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "$API_BASE/projects/$PROJECT_ENCODED")

PROJECT_ID=$(echo "$PROJECT_INFO" | jq -r '.id')
DEFAULT_BRANCH=$(echo "$PROJECT_INFO" | jq -r '.default_branch')

echo "   Project ID: $PROJECT_ID"
echo "   Default Branch: $DEFAULT_BRANCH"
echo ""

echo "📝 Step 2: Get latest commit on $DEFAULT_BRANCH..."
MAIN_REF=$(curl -s \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "$API_BASE/projects/$PROJECT_ID/repository/branches/$DEFAULT_BRANCH")

MAIN_SHA=$(echo "$MAIN_REF" | jq -r '.commit.id')
echo "   Latest SHA: ${MAIN_SHA:0:8}"
echo ""

# Create a unique branch name
TIMESTAMP=$(date +%s)
BRANCH_NAME="test/webhook-test-$TIMESTAMP"

echo "🌿 Step 3: Create test branch: $BRANCH_NAME..."
CREATE_BRANCH=$(curl -s -X POST \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "$API_BASE/projects/$PROJECT_ID/repository/branches?branch=$BRANCH_NAME&ref=$DEFAULT_BRANCH")

if echo "$CREATE_BRANCH" | jq -e '.name' > /dev/null 2>&1; then
    echo "   ✅ Branch created successfully"
else
    echo "   ❌ Failed to create branch"
    echo "$CREATE_BRANCH" | jq '.'
    exit 1
fi
echo ""

echo "📄 Step 4: Create a test file..."
FILE_CONTENT="# Test File\n\nCreated at: $(date)\nTimestamp: $TIMESTAMP\n"
CREATE_FILE=$(curl -s -X POST \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"branch\": \"$BRANCH_NAME\",
    \"commit_message\": \"Add test file for webhook test\",
    \"actions\": [
      {
        \"action\": \"create\",
        \"file_path\": \"workspace/test-$TIMESTAMP.txt\",
        \"content\": \"$FILE_CONTENT\"
      }
    ]
  }" \
  "$API_BASE/projects/$PROJECT_ID/repository/commits")

if echo "$CREATE_FILE" | jq -e '.id' > /dev/null 2>&1; then
    COMMIT_SHA=$(echo "$CREATE_FILE" | jq -r '.id')
    echo "   ✅ Commit created: ${COMMIT_SHA:0:8}"
else
    echo "   ❌ Failed to create commit"
    echo "$CREATE_FILE" | jq '.'
    exit 1
fi
echo ""

echo "🔀 Step 5: Create Merge Request..."
CREATE_MR=$(curl -s -X POST \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"source_branch\": \"$BRANCH_NAME\",
    \"target_branch\": \"$DEFAULT_BRANCH\",
    \"title\": \"🧪 Webhook Test MR - $TIMESTAMP\",
    \"description\": \"This MR is created by the Telefonistka webhook test script.\\n\\nIt tests webhook delivery and processing.\"
  }" \
  "$API_BASE/projects/$PROJECT_ID/merge_requests")

if echo "$CREATE_MR" | jq -e '.iid' > /dev/null 2>&1; then
    MR_IID=$(echo "$CREATE_MR" | jq -r '.iid')
    MR_URL=$(echo "$CREATE_MR" | jq -r '.web_url')
    echo "   ✅ MR created successfully!"
    echo "   MR #$MR_IID"
    echo "   URL: $MR_URL"
else
    echo "   ❌ Failed to create MR"
    echo "$CREATE_MR" | jq '.'
    exit 1
fi
echo ""

echo "⏳ Step 6: Wait for webhook delivery..."
sleep 3
echo ""

echo "💬 Step 7: Check for Telefonistka comment..."
COMMENTS=$(curl -s \
  -H "PRIVATE-TOKEN: $GITLAB_TOKEN" \
  "$API_BASE/projects/$PROJECT_ID/merge_requests/$MR_IID/notes")

TELEFONISTKA_COMMENTS=$(echo "$COMMENTS" | jq '[.[] | select(.body | contains("Telefonistka") or contains("🤖"))]')
COMMENT_COUNT=$(echo "$TELEFONISTKA_COMMENTS" | jq 'length')

if [ "$COMMENT_COUNT" -gt 0 ]; then
    echo "   ✅ Found $COMMENT_COUNT Telefonistka comment(s)!"
    echo ""
    echo "   Latest comment:"
    echo "$TELEFONISTKA_COMMENTS" | jq -r '.[0].body' | head -3
else
    echo "   ⚠️  No Telefonistka comments found yet (webhook may still be processing)"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Test Complete!"
echo ""
echo "MR Details:"
echo "  - Number: #$MR_IID"
echo "  - URL: $MR_URL"
echo "  - Branch: $BRANCH_NAME"
echo ""
echo "Next steps:"
echo "  1. Check the MR in GitLab for Telefonistka comments"
echo "  2. Merge the MR to test promotion workflow"
echo "  3. Verify promotion MRs are created"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
