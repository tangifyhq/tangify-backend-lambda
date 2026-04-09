#!/bin/bash
# Stop: ./stop-dynamodb-local.sh [delete]

if [[ $# -eq 0 ]]; then
  docker compose -f dynamodb-local-docker-compose.yml down
fi

if [[ $1 == "delete" ]]; then
  docker compose -f dynamodb-local-docker-compose.yml down -v
fi
