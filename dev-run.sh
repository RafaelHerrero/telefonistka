#!/bin/bash
# Development script to run Telefonistka locally
# This is useful for testing changes while developing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Telefonistka Development Runner ===${NC}"

# Check if .env file exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found!${NC}"
    echo -e "${YELLOW}Please create a .env file based on .env.example${NC}"
    echo -e "cp .env.example .env"
    echo -e "Then edit .env with your values"
    exit 1
fi

# Load environment variables
echo -e "${GREEN}Loading environment variables from .env${NC}"
export $(cat .env | grep -v '^#' | xargs)

# Check required variables
if [ -z "$GITHUB_OAUTH_TOKEN" ]; then
    echo -e "${RED}Error: GITHUB_OAUTH_TOKEN not set in .env${NC}"
    exit 1
fi

# Build the binary
echo -e "${GREEN}Building Telefonistka...${NC}"
go build -o telefonistka .

if [ $? -ne 0 ]; then
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi

echo -e "${GREEN}Build successful!${NC}"
echo -e "${YELLOW}Starting Telefonistka server on port 8080...${NC}"
echo -e "${YELLOW}Make sure ngrok is running: ngrok http 8080${NC}"
echo ""

# Run the server
./telefonistka server
