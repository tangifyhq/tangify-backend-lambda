# tangify-backend-lambda

Go AWS Lambda backend for Tangify (users/auth + billing + kitchen + plating).

## API documentation

- Main API reference: `api.md`
- Bruno collection: `apidocs-bruno/`

## Project layout

```bash
.
├── api/                         # Lambda handler and domain logic
│   ├── main.go                  # HTTP route handling
│   ├── go.mod
│   ├── users/
│   ├── billing/
│   └── menu/
├── dynamodb/
│   ├── users/                   # users table definition
│   └── billing/                 # sessions/orders/bills table definitions
├── create-dynamodb-tables.sh    # create required DynamoDB tables
├── deploy.sh                    # sam build + sam deploy
├── template.yaml                # SAM template
└── api.md                       # endpoint docs
```

## Prerequisites

- Go (matches `api/go.mod`, currently `go 1.25.0`)
- AWS CLI configured
- AWS SAM CLI
- Docker (needed by some SAM local workflows)

## Required AWS resources

The API expects these DynamoDB tables:

- `tangify_users`
- `tangify_sessions`
- `tangify_orders`
- `tangify_bills`

Create them from repo root:

```bash
./create-dynamodb-tables.sh
```

For DynamoDB local:

```bash
ENDPOINT_URL=http://localhost:8000 ./create-dynamodb-tables.sh
```

## Environment variables

Set these in Lambda (or local env):

- `TANGIFY_BOOTSTRAP_SECRET` - required for `POST /api/v1/users/bootstrap`
- `TANGIFY_VENUE_ID` - default venue when request does not pass one (default fallback is `default`)
- `GOOGLE_SHEETS_API_KEY` - required for `GET /api/v1/menu`
- `GOOGLE_SHEET_ID` - required for `GET /api/v1/menu`
- `GOOGLE_SHEET_NAME` - optional for `GET /api/v1/menu`

Also ensure SSM contains:

- `tangify.jwt.secret` (used to sign/verify JWTs)

## Build and test

Build API package:

```bash
cd api
go build ./...
```

Run tests:

```bash
cd api
go test ./...
```

## Local run (SAM)

From repo root:

```bash
sam build
sam local start-api
```

## Deploy

Quick deploy script:

```bash
./deploy.sh
```

Or manually:

```bash
sam build
sam deploy --guided
```
