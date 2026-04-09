#!/bin/bash

# Create DynamoDB table in DynamoDB Local
aws dynamodb create-table \
  --endpoint-url http://localhost:8000 \
  --table-name tangify_bills \
  --attribute-definitions \
    AttributeName=id,AttributeType=S \
    AttributeName=payment_status,AttributeType=S \
  --key-schema \
    AttributeName=id,KeyType=HASH \
    AttributeName=payment_status,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST

echo "Table 'tangify_bills' created successfully in DynamoDB Local"

