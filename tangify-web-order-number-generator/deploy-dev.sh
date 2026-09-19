#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "Installing dependencies (if needed)..."
npm install

echo "Running Wrangler dry-run (dev)..."
npx wrangler deploy --dry-run --env dev

echo "Deploying dev Worker..."
npx wrangler deploy --env dev

echo "Dev deployment complete."
echo "Worker URL: https://tangify-web-order-number-generator-dev.subnub.workers.dev/"
