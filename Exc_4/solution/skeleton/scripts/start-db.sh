#!/bin/bash
docker volume create pgdata

docker run -d \
  --name postgres-db \
  --network host \
  --env-file debug.env \
  -v pgdata:/var/lib/postgresql/18/docker \
  postgres:18