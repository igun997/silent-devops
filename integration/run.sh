#!/bin/sh
set -eu
compose='docker compose -f integration/docker-compose.yml'
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then $compose ps || true; $compose logs --no-color || true; fi
  $compose down -v --remove-orphans || true
  exit "$status"
}
trap cleanup EXIT INT TERM
$compose build
$compose up -d --wait
go test -tags=e2e ./integration/... -count=1 -v
