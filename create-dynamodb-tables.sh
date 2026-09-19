#!/usr/bin/env bash
# Create DynamoDB tables used by the API: billing (sessions, orders, bills) and users.
# Billing shapes match api/billing/billing_models_v2.go — item attributes must include
# GSI key fields (e.g. venue_id, opened_at on sessions; venue_id on orders).
# Users table shape is dynamodb/users/tangify_users.json (see api/users/).
#
# Usage:
#   ./create-dynamodb-tables.sh                    # AWS default endpoint / credentials
#   ENDPOINT_URL=http://localhost:8000 ./create-dynamodb-tables.sh   # DynamoDB Local
#
# Env:
#   ENDPOINT_URL  — if set, passed to --endpoint-url (e.g. http://localhost:8000)
#   AWS_REGION    — defaults to ap-south-1
#   AWS_PROFILE   — optional

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BILLING_JSON_DIR="${SCRIPT_DIR}/dynamodb/billing"
USERS_JSON_DIR="${SCRIPT_DIR}/dynamodb/users"
LOYALTY_JSON_DIR="${SCRIPT_DIR}/dynamodb/loyalty"

REGION="${AWS_REGION:-ap-south-1}"
export AWS_DEFAULT_REGION="${REGION}"

AWS_EXTRA=()
if [[ -n "${ENDPOINT_URL:-}" ]]; then
  AWS_EXTRA+=(--endpoint-url "${ENDPOINT_URL}")
fi

aws_ddb() {
  # Global options (--endpoint-url) belong immediately after `aws`, not after `dynamodb`.
  if [[ ${#AWS_EXTRA[@]} -gt 0 ]]; then
    aws "${AWS_EXTRA[@]}" dynamodb "$@"
  else
    aws dynamodb "$@"
  fi
}

table_exists() {
  local table_name="$1"
  aws_ddb describe-table --table-name "${table_name}" &>/dev/null
}

create_one() {
  local json_dir="$1"
  local name="$2"
  local file="${json_dir}/${name}.json"
  if [[ ! -f "${file}" ]]; then
    echo "error: missing ${file}" >&2
    exit 1
  fi
  local table_name
  table_name="$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['TableName'])" "${file}")"

  if table_exists "${table_name}"; then
    echo "Table '${table_name}' already exists — skipping."
    return 0
  fi

  echo "Creating table '${table_name}' from ${file} ..."
  aws_ddb create-table --cli-input-json "file://${file}"
}

create_one "${BILLING_JSON_DIR}" tangify_sessions
create_one "${BILLING_JSON_DIR}" tangify_orders
create_one "${BILLING_JSON_DIR}" tangify_bills
create_one "${BILLING_JSON_DIR}" tangify_bills_with_line_items
create_one "${BILLING_JSON_DIR}" dev-tangify_bills_with_line_items
create_one "${USERS_JSON_DIR}" tangify_users
create_one "${LOYALTY_JSON_DIR}" tangify_points_wallet
create_one "${LOYALTY_JSON_DIR}" dev-tangify_points_wallet
create_one "${LOYALTY_JSON_DIR}" tangify_phone_otp
if table_exists "tangify_phone_otp"; then
  aws_ddb update-time-to-live \
    --table-name "tangify_phone_otp" \
    --time-to-live-specification "Enabled=true,AttributeName=ttl" \
    >/dev/null || true
fi

WEBORDERS_JSON_DIR="${SCRIPT_DIR}/dynamodb/weborders"
create_one "${WEBORDERS_JSON_DIR}" tangify-login-nonce
if table_exists "tangify-login-nonce"; then
  aws_ddb update-time-to-live \
    --table-name "tangify-login-nonce" \
    --time-to-live-specification "Enabled=true,AttributeName=ttl" \
    >/dev/null || true
fi
create_one "${WEBORDERS_JSON_DIR}" tangify-web-orders
create_one "${WEBORDERS_JSON_DIR}" tangify-web-addresses

echo ""
echo "Done. Tables: tangify_sessions, tangify_orders, tangify_bills, tangify_bills_with_line_items, dev-tangify_bills_with_line_items, tangify_users, tangify_points_wallet, dev-tangify_points_wallet, tangify_phone_otp, tangify-login-nonce, tangify-web-orders, tangify-web-addresses"
if [[ -n "${ENDPOINT_URL:-}" ]]; then
  echo "List: aws dynamodb list-tables --endpoint-url \"${ENDPOINT_URL}\" --region ${REGION}"
else
  echo "List: aws dynamodb list-tables --region ${REGION}"
fi
