#!/bin/bash

# Replace YOUR_FUNCTION_URL with your actual Lambda Function URL
# Replace YOUR_JWT_TOKEN with your actual JWT token

FUNCTION_URL="YOUR_FUNCTION_URL"
JWT_TOKEN="YOUR_JWT_TOKEN"

# POST /api/v1/billing/order - Create a new bill with an order
curl -X POST "${FUNCTION_URL}/api/v1/billing/order" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d '{
    "items": [
      {
        "name": "Pizza Margherita",
        "quantity": 2,
        "price": 50000
      },
      {
        "name": "Coca Cola",
        "quantity": 1,
        "price": 5000
      }
    ],
    "table_id": "table_123",
    "customer_id": "customer_456",
    "staff_id": "staff_789"
  }'

# POST /api/v1/billing/order - Add order to existing bill
curl -X POST "${FUNCTION_URL}/api/v1/billing/order" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d '{
    "bill_id": "bill_existing_123",
    "items": [
      {
        "name": "Dessert",
        "quantity": 1,
        "price": 15000
      }
    ],
    "table_id": "table_123",
    "customer_id": "customer_456",
    "staff_id": "staff_789"
  }'

