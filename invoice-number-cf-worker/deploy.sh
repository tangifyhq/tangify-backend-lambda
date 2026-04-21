#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "Installing dependencies (if needed)..."
npm install

echo "Running Wrangler dry-run..."
npx wrangler deploy --dry-run

echo "Deploying Worker..."
npx wrangler deploy

echo "Deployment complete."
