#!/usr/bin/env bash
# Create billing v2 DynamoDB tables (sessions, orders, bills) via AWS CLI.
# Matches shapes in api/billing/billing_models_v2.go — item attributes must include
# GSI key fields (e.g. venue_id, opened_at on sessions; venue_id on orders).
#
# Usage:
#   ./create-dynamodb-billing-tables-v2.sh                    # AWS default endpoint / credentials
#   ENDPOINT_URL=http://localhost:8000 ./create-dynamodb-billing-tables-v2.sh   # DynamoDB Local
#
# Env:
#   ENDPOINT_URL  — if set, passed to --endpoint-url (e.g. http://localhost:8000)
#   AWS_REGION    — defaults to ap-south-1
#   AWS_PROFILE   — optional

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JSON_DIR="${SCRIPT_DIR}/dynamodb/billing"

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
  local name="$1"
  local file="${JSON_DIR}/${name}.json"
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

create_one tangify_sessions
create_one tangify_orders
create_one tangify_bills

echo ""
echo "Done. Tables: tangify_sessions, tangify_orders, tangify_bills"
if [[ -n "${ENDPOINT_URL:-}" ]]; then
  echo "List: aws dynamodb list-tables --endpoint-url \"${ENDPOINT_URL}\" --region ${REGION}"
else
  echo "List: aws dynamodb list-tables --region ${REGION}"
fi
