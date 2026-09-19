#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${1:-$ROOT/.env.lambda}"
FUNCTION_NAME="${LAMBDA_FUNCTION_NAME:-tangify-backend-lambda}"
AWS_REGION="${AWS_REGION:-ap-south-1}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE"
  echo "Copy .env.lambda.example to .env.lambda and fill in values."
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "$ENV_FILE"
set +a

required=(LLM_API_KEY GOOGLE_SHEETS_API_KEY GOOGLE_SHEET_ID)
for key in "${required[@]}"; do
  if [[ -z "${!key:-}" ]]; then
    echo "Missing required env var in $ENV_FILE: $key"
    exit 1
  fi
done

vars=(
  "LLM_API_KEY=${LLM_API_KEY}"
  "GOOGLE_SHEETS_API_KEY=${GOOGLE_SHEETS_API_KEY}"
  "GOOGLE_SHEET_ID=${GOOGLE_SHEET_ID}"
)

if [[ -n "${GOOGLE_SHEET_NAME:-}" ]]; then
  vars+=("GOOGLE_SHEET_NAME=${GOOGLE_SHEET_NAME}")
fi
if [[ -n "${GOOGLE_ORDERING_SHEET_NAME:-}" ]]; then
  vars+=("GOOGLE_ORDERING_SHEET_NAME=${GOOGLE_ORDERING_SHEET_NAME}")
fi
if [[ -n "${TANGIFY_BOOTSTRAP_SECRET:-}" ]]; then
  vars+=("TANGIFY_BOOTSTRAP_SECRET=${TANGIFY_BOOTSTRAP_SECRET}")
fi
if [[ -n "${TANGIFY_VENUE_ID:-}" ]]; then
  vars+=("TANGIFY_VENUE_ID=${TANGIFY_VENUE_ID}")
fi
if [[ -n "${CF_SECRET:-}" ]]; then
  vars+=("CF_SECRET=${CF_SECRET}")
fi
if [[ -n "${ABLY_KEY:-}" ]]; then
  vars+=("ABLY_KEY=${ABLY_KEY}")
fi
if [[ -n "${ABLY_CHANNEL:-}" ]]; then
  vars+=("ABLY_CHANNEL=${ABLY_CHANNEL}")
fi
if [[ -n "${ABLY_CHANNEL_DEV:-}" ]]; then
  vars+=("ABLY_CHANNEL_DEV=${ABLY_CHANNEL_DEV}")
fi
if [[ -n "${INVOICE_NUMBER_WORKER_URL_PROD:-}" ]]; then
  vars+=("INVOICE_NUMBER_WORKER_URL_PROD=${INVOICE_NUMBER_WORKER_URL_PROD}")
fi
if [[ -n "${INVOICE_NUMBER_WORKER_URL_DEV:-}" ]]; then
  vars+=("INVOICE_NUMBER_WORKER_URL_DEV=${INVOICE_NUMBER_WORKER_URL_DEV}")
fi
if [[ -n "${WEB_ORDER_NUMBER_WORKER_URL_PROD:-}" ]]; then
  vars+=("WEB_ORDER_NUMBER_WORKER_URL_PROD=${WEB_ORDER_NUMBER_WORKER_URL_PROD}")
fi
if [[ -n "${WEB_ORDER_NUMBER_WORKER_URL_DEV:-}" ]]; then
  vars+=("WEB_ORDER_NUMBER_WORKER_URL_DEV=${WEB_ORDER_NUMBER_WORKER_URL_DEV}")
fi
if [[ -n "${GUPSHUP_API_KEY:-}" ]]; then
  vars+=("GUPSHUP_API_KEY=${GUPSHUP_API_KEY}")
fi
if [[ -n "${GUPSHUP_SOURCE:-}" ]]; then
  vars+=("GUPSHUP_SOURCE=${GUPSHUP_SOURCE}")
fi
if [[ -n "${GUPSHUP_APP_NAME:-}" ]]; then
  vars+=("GUPSHUP_APP_NAME=${GUPSHUP_APP_NAME}")
fi
if [[ -n "${GUPSHUP_REWARD_POINT_TEMPLATE_ID:-}" ]]; then
  vars+=("GUPSHUP_REWARD_POINT_TEMPLATE_ID=${GUPSHUP_REWARD_POINT_TEMPLATE_ID}")
fi
if [[ -n "${GUPSHUP_POINTS_USED_TEMPLATE_ID:-}" ]]; then
  vars+=("GUPSHUP_POINTS_USED_TEMPLATE_ID=${GUPSHUP_POINTS_USED_TEMPLATE_ID}")
fi
if [[ -n "${GUPSHUP_OTP_TEMPLATE_ID:-}" ]]; then
  vars+=("GUPSHUP_OTP_TEMPLATE_ID=${GUPSHUP_OTP_TEMPLATE_ID}")
fi
if [[ -n "${R2_ACCOUNT_ID:-}" ]]; then
  vars+=("R2_ACCOUNT_ID=${R2_ACCOUNT_ID}")
fi
if [[ -n "${R2_ACCESS_KEY_ID:-}" ]]; then
  vars+=("R2_ACCESS_KEY_ID=${R2_ACCESS_KEY_ID}")
fi
if [[ -n "${R2_SECRET_ACCESS_KEY:-}" ]]; then
  vars+=("R2_SECRET_ACCESS_KEY=${R2_SECRET_ACCESS_KEY}")
fi
if [[ -n "${R2_BUCKET:-}" ]]; then
  vars+=("R2_BUCKET=${R2_BUCKET}")
fi
if [[ -n "${R2_MENU_KEY:-}" ]]; then
  vars+=("R2_MENU_KEY=${R2_MENU_KEY}")
fi
if [[ -n "${WEB_MENU_PUBLIC_BASE_URL:-}" ]]; then
  vars+=("WEB_MENU_PUBLIC_BASE_URL=${WEB_MENU_PUBLIC_BASE_URL}")
fi
if [[ -n "${WEB_ORDER_PUBLIC_BASE_URL:-}" ]]; then
  vars+=("WEB_ORDER_PUBLIC_BASE_URL=${WEB_ORDER_PUBLIC_BASE_URL}")
fi
if [[ -n "${CF_API_TOKEN:-}" ]]; then
  vars+=("CF_API_TOKEN=${CF_API_TOKEN}")
fi
if [[ -n "${CF_ZONE_ID:-}" ]]; then
  vars+=("CF_ZONE_ID=${CF_ZONE_ID}")
fi
if [[ -n "${RAZORPAY_KEY_ID:-}" ]]; then
  vars+=("RAZORPAY_KEY_ID=${RAZORPAY_KEY_ID}")
fi
if [[ -n "${RAZORPAY_KEY_SECRET:-}" ]]; then
  vars+=("RAZORPAY_KEY_SECRET=${RAZORPAY_KEY_SECRET}")
fi
if [[ -n "${SHIPROCKET_EMAIL:-}" ]]; then
  vars+=("SHIPROCKET_EMAIL=${SHIPROCKET_EMAIL}")
fi
if [[ -n "${SHIPROCKET_PASSWORD:-}" ]]; then
  vars+=("SHIPROCKET_PASSWORD=${SHIPROCKET_PASSWORD}")
fi
if [[ -n "${SHIPROCKET_PICKUP_POSTCODE:-}" ]]; then
  vars+=("SHIPROCKET_PICKUP_POSTCODE=${SHIPROCKET_PICKUP_POSTCODE}")
fi
if [[ -n "${SHIPROCKET_PICKUP_LAT:-}" ]]; then
  vars+=("SHIPROCKET_PICKUP_LAT=${SHIPROCKET_PICKUP_LAT}")
fi
if [[ -n "${SHIPROCKET_PICKUP_LNG:-}" ]]; then
  vars+=("SHIPROCKET_PICKUP_LNG=${SHIPROCKET_PICKUP_LNG}")
fi
if [[ -n "${SHIPROCKET_DEFAULT_WEIGHT_KG:-}" ]]; then
  vars+=("SHIPROCKET_DEFAULT_WEIGHT_KG=${SHIPROCKET_DEFAULT_WEIGHT_KG}")
fi

joined=$(IFS=, ; echo "${vars[*]}")

echo "Updating Lambda env vars on $FUNCTION_NAME ($AWS_REGION)..."
aws lambda update-function-configuration \
  --function-name "$FUNCTION_NAME" \
  --region "$AWS_REGION" \
  --environment "Variables={$joined}" \
  >/dev/null

echo "Done. Current keys:"
aws lambda get-function-configuration \
  --function-name "$FUNCTION_NAME" \
  --region "$AWS_REGION" \
  --query 'Environment.Variables | keys(@)' \
  --output json
