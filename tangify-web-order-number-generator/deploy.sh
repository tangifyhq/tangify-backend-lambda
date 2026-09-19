#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

echo "Installing dependencies (if needed)..."
npm install

echo "Running Wrangler dry-run..."
npx wrangler deploy --dry-run --env=""

echo "Deploying Worker..."
npx wrangler deploy --env=""

echo "Deployment complete."
echo "Worker URL: https://tangify-web-order-number-generator.subnub.workers.dev/"
