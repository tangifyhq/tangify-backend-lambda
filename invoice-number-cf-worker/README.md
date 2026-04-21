# invoice-number-cf-worker

Cloudflare Worker that generates invoice numbers from a `bill_id`.

## Behavior

- `POST /` with body `{ "bill_id": "bill_123" }`
- Resolves current UTC year (for example, `2026`)
- Routes request to Durable Object instance keyed by that year
- Durable Object allocates auto-increment sequence for that year
- Stores both mappings in Durable Object storage in one transaction:
  - `bill:{bill_id}` -> invoice payload
  - `inv:{invoice_number}` -> invoice payload
- Returns:
  - `invoice_number`
  - `bill_id`
  - `year`
  - `sequence`

If `bill_id` already has an invoice, it returns existing mapping (idempotent by bill).

## Setup

1. Install dependencies:

```bash
npm install
```

2. Run locally:

```bash
npm run dev
```

3. Deploy:

```bash
npm run deploy
```
