# Telefonistka Test Repository

This is a test repository to learn and test Telefonistka's promotion workflow.

## Repository Structure

```
.
├── telefonistka.yaml          # Telefonistka configuration
├── workspace/                 # Source of truth - your changes start here
│   └── demo-app/
│       ├── deployment.yaml
│       └── values.yaml
└── clusters/                  # Promotion targets
    ├── dev/
    │   └── demo-app/
    └── staging/
        └── demo-app/
```

## How It Works

1. **Make changes in `workspace/demo-app/`** - This is where you start
2. **Create a PR** to merge to main branch
3. **Telefonistka automatically creates promotion PRs**:
   - First PR: Promotes to `clusters/dev/`
   - Second PR: Promotes from `dev` to `clusters/staging/`

## Setup Steps

### 1. Create GitHub Repository

```bash
# From this directory
git init
git add .
git commit -m "Initial commit"
gh repo create test-telefonistka --public --source=. --remote=origin --push
```

Or manually create a repo on GitHub and push this directory.

### 2. Configure GitHub Token

Create a Personal Access Token with `repo` permissions:
- Go to: https://github.com/settings/tokens
- Click "Generate new token" → "Generate new token (classic)"
- Select scope: `repo` (full control)
- Copy the token

### 3. Configure Webhook (for local development)

If running Telefonistka locally with ngrok:

1. Start ngrok: `ngrok http 8080`
2. Copy the HTTPS URL (e.g., `https://abc123.ngrok.io`)
3. Go to repo Settings → Webhooks → Add webhook
   - Payload URL: `https://abc123.ngrok.io/webhook`
   - Content type: `application/json`
   - Secret: Generate a random string and save it
   - Events: Select "Pull requests"

### 4. GitHub Actions Alternative (No ngrok needed)

If you prefer using GitHub Actions (easier but slower):

1. Go to repo Settings → Actions → General
2. Set Workflow permissions to "Read and write permissions"
3. The workflow file is already in `.github/workflows/telefonistka.yaml`
4. Add your GitHub token as a secret if needed

## Testing the Flow

1. **Make a change in workspace**:
   ```bash
   cd workspace/demo-app
   # Edit values.yaml - change the image tag or replica count
   git checkout -b test-change
   git add .
   git commit -m "Update demo-app"
   git push origin test-change
   ```

2. **Create a PR** to main branch

3. **Merge the PR** and watch Telefonistka:
   - Create a promotion PR to dev
   - After merging dev PR, create promotion PR to staging

## What to Look For

- Check PR comments for promotion information
- Look for drift warnings if environments are out of sync
- See the promotion "flow" annotation in PRs

## Next Steps

- Read the main [DEVELOPMENT.md](../DEVELOPMENT.md) for local development setup
- Modify `telefonistka.yaml` to experiment with different promotion flows
- Add more environments or change the promotion path
