# Tangify backend HTTP API

Base URL is your Lambda Function URL or API Gateway stage URL (examples use `https://example.lambda-url.us-east-1.on.aws`).

- **Public (no JWT):** `GET /api/v1/health`, `GET /api/v1/menu`, `POST /api/v1/auth/login`, `POST /api/v1/users/bootstrap` (bootstrap requires `X-Bootstrap-Secret` header)  
- **Protected:** other routes require header `Authorization: Bearer <JWT>`  

Successful responses return **JSON bodies directly** (no `{ "data": ... }` wrapper). Errors use  
`{ "error": "<message>" }` with appropriate HTTP status (`400`, `401`, `500`, etc.).

---

## Health

### `GET /api/v1/health`

**Response** `200`

```json
{ "status": "ok" }
```

```bash
curl -sS "https://EXAMPLE.lambda-url.on.aws/api/v1/health"
```

---

## Menu (Google Sheet)

### `GET /api/v1/menu`

Loads menu rows from Google Sheets (server-side env: `GOOGLE_SHEETS_API_KEY`, `GOOGLE_SHEET_ID`, `GOOGLE_SHEET_NAME`).

**Response** `200` — array of items:

```json
[
  {
    "status": "ON",
    "category": "Mains",
    "name": "Dal Tadka",
    "description": "",
    "is_veg": true,
    "price": "180"
  }
]
```

```bash
curl -sS "https://EXAMPLE.lambda-url.on.aws/api/v1/menu"
```

---

## Users & auth

Stored in DynamoDB table `tangify_users` (see `dynamodb/users/tangify_users.json`). Passwords are **bcrypt** hashes; API never returns `pw_hash`.

**Roles:** `waiter`, `kitchen`, `admin`.

JWT claims include `identity` (user id), `name`, `role`, and standard `exp` / `iat` (24h TTL).

### `POST /api/v1/auth/login`

**Request body** — `LoginResponse` input:

```json
{ "login": "user@example.com", "password": "secret" }
```

`login` may be **email** (contains `@`) or **phone** (normalized: spaces stripped).

**Response** `200` — `LoginResponse`:

```json
{
  "token": "<jwt>",
  "user": {
    "id": "…",
    "phone": "+9198…",
    "email": "user@example.com",
    "name": "Ravi",
    "role": "waiter"
  }
}
```

**Errors:** `401` invalid credentials.

```bash
curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"login":"admin@example.com","password":"your-password"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/auth/login"
```

---

### `POST /api/v1/users/bootstrap`

Creates the first user (typically **admin**) when `TANGIFY_BOOTSTRAP_SECRET` is set in the environment. **Does not require JWT.**  
Header: `X-Bootstrap-Secret: <same as env>`.

**Request body** — `BootstrapUserRequest`:

```json
{
  "phone": "+919876543210",
  "email": "admin@example.com",
  "name": "Admin",
  "role": "admin",
  "password": "long-secure-password"
}
```

**Response** `200` — `UserPublic` (same shape as `user` in login).

```bash
curl -sS -X POST -H "Content-Type: application/json" \
  -H "X-Bootstrap-Secret: $TANGIFY_BOOTSTRAP_SECRET" \
  -d '{"phone":"+919876543210","email":"admin@example.com","name":"Admin","role":"admin","password":"securepass123"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/users/bootstrap"
```

---

### `POST /api/v1/users`

**Admin only.** Creates a user with a known password.

**Request body** — `CreateUserRequest`:

```json
{
  "phone": "+919876543210",
  "email": "waiter1@example.com",
  "name": "Waiter One",
  "role": "waiter",
  "password": "long-secure-password"
}
```

**Response** `200` — `UserPublic`.

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"phone":"+919876543210","email":"waiter1@example.com","name":"Waiter One","role":"waiter","password":"securepass123"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/users"
```

---

### `GET /api/v1/users/me`

Returns the authenticated user (from JWT `identity`).

**Response** `200` — `UserPublic` (same as login `user`).

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/users/me"
```

---

### `PATCH /api/v1/users/password`

**Rules:**

- **Self:** must send `current_password` and `new_password`; `user_id` must be your own id.
- **Admin** changing **another** user: send `user_id` and `new_password` only; `current_password` is not required.

**Request body** — `ChangePasswordRequest`:

```json
{
  "user_id": "<uuid>",
  "current_password": "old",
  "new_password": "new-long-password"
}
```

**Response** `200`: `{ "status": "ok" }`.

```bash
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"user_id":"<uuid>","current_password":"old","new_password":"newsecurepass123"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/users/password"
```

---

## Billing — waiter

Default venue for reads/writes is `TANGIFY_VENUE_ID` env or `"default"`. Sessions and orders store `venue_id` for DynamoDB GSIs.

### Order channels (`channel`)

| Value | Description |
|--------|----------------|
| `dining_table` | In-restaurant table |
| `takeaway` | Takeaway |
| `whatsapp_quickdelivery` | WhatsApp quick delivery |
| `whatsapp_normaldelivery` | WhatsApp normal delivery |
| `neighbour_delivery` | Neighbour delivery |

### `GET /api/v1/billing/live-orders`

Live or billing sessions with their orders (waiter board).

**Query**

| Param | Required | Description |
|--------|----------|-------------|
| `venue_id` | No | Defaults to server default venue |

**Response** `200` — `LiveOrdersGroupedResponse`:

```json
{
  "sessions": [
    {
      "session": {
        "id": "sess_…",
        "table_ids": ["T1", "T2"],
        "status": "live",
        "bill_id": "",
        "opened_at": 1710000000000,
        "venue_id": "default"
      },
      "orders": []
    }
  ]
}
```

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/live-orders?venue_id=default"
```

---

### `POST /api/v1/billing/sessions`

Open a table session and place the **first** order (tables become live).

**Request body** — `CreateSessionAndFirstOrderRequest`:

```json
{
  "table_ids": ["T5"],
  "items": [
    { "id": "", "name": "Dal", "quantity": 2, "price": 18000, "status": "" }
  ],
  "channel": "dining_table",
  "customer_id": null,
  "staff_id": null,
  "ordered_at": null
}
```

- `table_ids`: one table or multiple for **joined** tables.  
- Line `id` / `status` may be omitted; server fills defaults (`line_*` ids, `queued`).  
- `price` is in **paise** (integer). Totals use `sum(price * quantity)` per order.

**Response** `200` — `SessionWithOrders`:

```json
{
  "session": {
    "id": "sess_…",
    "table_ids": ["T5"],
    "status": "live",
    "opened_at": 1710000000000,
    "updated_at": 1710000000000,
    "venue_id": "default"
  },
  "orders": [
    {
      "id": "ord_…",
      "session_id": "sess_…",
      "venue_id": "default",
      "channel": "dining_table",
      "items": [{ "id": "line_…", "name": "Dal", "quantity": 2, "price": 18000, "status": "queued" }],
      "total_price": 36000,
      "kitchen_status": "pending",
      "ordered_at": 1710000000000,
      "updated_at": 1710000000000
    }
  ]
}
```

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"table_ids":["T5"],"items":[{"name":"Dal","quantity":2,"price":18000}],"channel":"dining_table"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/sessions"
```

---

### `POST /api/v1/billing/orders`

Add another order to an existing session.

**Request body** — `AddOrderToSessionRequest`:

```json
{
  "session_id": "sess_…",
  "items": [{ "name": "Rice", "quantity": 1, "price": 8000 }],
  "channel": "dining_table",
  "source_table_id": null,
  "staff_id": null,
  "ordered_at": null
}
```

**Response** `200` — `Order` (single object).

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"session_id":"sess_xxx","items":[{"name":"Rice","quantity":1,"price":8000}],"channel":"dining_table"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/orders"
```

---

### `PATCH /api/v1/billing/orders`

Update line items and/or kitchen status on an order.

**Request body** — `UpdateOrderRequestV2`:

```json
{
  "order_id": "ord_…",
  "items": [{ "id": "line_…", "name": "Dal", "quantity": 2, "price": 18000, "status": "queued" }],
  "total_price": null,
  "kitchen_status": null
}
```

Omit `items` to leave lines unchanged; set `kitchen_status` to a **KitchenStatus** value if needed (`pending`, `preparing`, `ready`, `served`, `cancelled`).

**Response** `200` — `Order`.

```bash
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"ord_xxx","kitchen_status":"preparing"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/orders"
```

---

### `GET /api/v1/billing/orders`

List orders either by session or by physical table.

**Query (one required)**

| Param | Description |
|--------|-------------|
| `session_id` | All orders for this session |
| `table_id` | Orders for the **live/billing** session that contains this table |
| `venue_id` | With `table_id`, optional venue (defaults server-side) |

**Response** `200` — `Order[]`.

```bash
# By session
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/orders?session_id=sess_xxx"

# By table
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/orders?table_id=T5&venue_id=default"
```

---

### `POST /api/v1/billing/bills/start`

Create the bill and move session to **billing**; links orders to the bill and rolls up totals.

**Request body** — `StartBillForSessionRequest`:

```json
{ "session_id": "sess_…", "staff_id": null }
```

**Response** `200` — `Bill`:

```json
{
  "id": "bill_…",
  "session_id": "sess_…",
  "table_ids": ["T5"],
  "payment_method": "cash",
  "payment_status": "pending",
  "created_at": 1710000000000,
  "updated_at": 1710000000000,
  "total_tax_in_paise": 0,
  "total_discount_in_paise": 0,
  "total_amount_in_paise": 44000
}
```

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"session_id":"sess_xxx"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/bills/start"
```

---

### `PATCH /api/v1/billing/bills`

Update discounts, taxes, payment fields, or total.

**Request body** — `UpdateBillRequestV2`:

```json
{
  "bill_id": "bill_…",
  "discounts": [],
  "taxes": [],
  "payment_method": "card",
  "payment_status": "pending",
  "total_amount_in_paise": null
}
```

**Response** `200` — `Bill`.

```bash
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"bill_id":"bill_xxx","payment_method":"card"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/bills"
```

---

### `POST /api/v1/billing/sessions/close`

Finalize checkout: mark bill paid and session **closed**.

**Request body** — `CloseTableRequest`:

```json
{ "session_id": "sess_…", "bill_id": "bill_…" }
```

**Response** `200`:

```json
{ "status": "closed" }
```

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"session_id":"sess_xxx","bill_id":"bill_xxx"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/billing/sessions/close"
```

---

## Kitchen

### `GET /api/v1/kitchen/item-board`

Expand all venue orders into per-line rows (excludes lines already `served` or `cancelled`).

**Query**

| Param | Required |
|--------|----------|
| `venue_id` | No (server default) |

**Response** `200` — `KitchenDishCount[]`:

```json
[
  {
    "order_id": "ord_…",
    "line_item_id": "line_…",
    "name": "Dal",
    "quantity": 2,
    "status": "queued"
  }
]
```

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/kitchen/item-board?venue_id=default"
```

---

### `PATCH /api/v1/kitchen/line-items/status`

Update **one line item**’s kitchen status.

**Request body** — `PatchLineItemStatusRequest`:

```json
{
  "order_id": "ord_…",
  "line_item_id": "line_…",
  "status": "preparing"
}
```

Line item statuses: `queued`, `preparing`, `ready`, `served`, `cancelled`.

**Response** `200` — full `Order` after update.

```bash
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"ord_xxx","line_item_id":"line_xxx","status":"ready"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/kitchen/line-items/status"
```

---

## Plating

### `GET /api/v1/plating/orders`

FIFO-style queue for plating: orders sorted by `ordered_at`, excluding orders whose **order-level** `kitchen_status` is `served`.

**Query**

| Param | Required | Description |
|--------|----------|-------------|
| `session_id` | One of `session_id` or `table_id` | FIFO for this session |
| `table_id` | One of above | Resolve live session for table, then FIFO |
| `venue_id` | No | Used with `table_id` (default venue) |
| `limit` | No | Max orders (default `100`) |

**Response** `200` — `PlatingQueueOrder[]`:

```json
[
  {
    "order_id": "ord_…",
    "session_id": "sess_…",
    "table_ids": ["T5"],
    "kitchen_status": "pending",
    "ordered_at": 1710000000000
  }
]
```

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/plating/orders?table_id=T5&venue_id=default&limit=50"
```

---

### `PATCH /api/v1/plating/orders/status`

Update **order-level** kitchen status (plating / expediter).

**Request body** — `PatchOrderKitchenStatusRequestV2`:

```json
{ "order_id": "ord_…", "kitchen_status": "ready" }
```

**Response** `200` — `Order`.

```bash
curl -sS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"order_id":"ord_xxx","kitchen_status":"served"}' \
  "https://EXAMPLE.lambda-url.on.aws/api/v1/plating/orders/status"
```

---

## Auth note

`POST /api/v1/auth/login` is listed as a **whitelisted** path (no JWT). Implement login in this service or another function; until then, requests to that path may fall through to the default handler. Obtain a JWT from your actual auth flow and pass it as `Authorization: Bearer …` for billing, kitchen, and plating routes.

---

## Default route

Any unmatched path returns `200` with:

```json
{ "message": "Hello, World!" }
```

(Useful only for smoke tests; prefer explicit routes above.)
