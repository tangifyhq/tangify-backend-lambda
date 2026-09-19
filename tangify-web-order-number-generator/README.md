# tangify-web-order-number-generator

Cloudflare Worker that allocates sequential web order refs from an `attempt_id`.

Separate from `invoice-number-cf-worker` (POS bill invoices). Prod and dev use
**separate Durable Object namespaces**.

Lambda / ordering-web wiring is not connected yet — deploy and smoke-test the
worker first.

## Workers

| Environment | Worker name | Expected URL |
|-------------|-------------|--------------|
| Production | `tangify-web-order-number-generator` | `https://tangify-web-order-number-generator.subnub.workers.dev/` |
| Dev | `tangify-web-order-number-generator-dev` | `https://tangify-web-order-number-generator-dev.subnub.workers.dev/` |

## Behavior

- `POST /` with body `{ "attempt_id": "uuid-or-stable-key" }`
- Resolves current UTC year (for example, `2026`)
- Routes to a Durable Object instance keyed by that year
- Allocates an auto-increment sequence for that year
- Stores both mappings in one storage transaction:
  - `attempt:{attempt_id}` → payload
  - `ord:{order_ref}` → payload
- Returns:
  - `order_ref` (for example, `TGFY-W-2026-000001`)
  - `attempt_id`
  - `year`
  - `sequence`

Retrying the same `attempt_id` returns the same mapping (idempotent).

## Setup

```bash
cd tangify-web-order-number-generator
npm install
```

Local:

```bash
npm run dev          # prod config
npm run dev:worker   # [env.dev] config
```

## Deploy

Production:

```bash
npm run deploy
# or
./deploy.sh
```

Dev (separate sequence):

```bash
npm run deploy:dev
# or
./deploy-dev.sh
```

## Smoke test

```bash
curl -s -X POST https://tangify-web-order-number-generator.subnub.workers.dev/ \
  -H 'Content-Type: application/json' \
  -d '{"attempt_id":"smoke_001"}'
```

Retry with the same `attempt_id` — response should be identical.

## Project layout

```bash
tangify-web-order-number-generator/
├── src/index.ts       # Worker + WebOrderYearCounter Durable Object
├── wrangler.toml      # prod + [env.dev]
├── deploy.sh
└── deploy-dev.sh
```
