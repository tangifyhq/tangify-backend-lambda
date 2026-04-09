#!/bin/bash
# Tail logs: ./dynamodb-local-logs-tail.sh

docker compose -f dynamodb-local-docker-compose.yml logs -f