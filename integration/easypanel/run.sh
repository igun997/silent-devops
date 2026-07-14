#!/bin/sh
# E2E for easypanel-migrate: two fake EasyPanel panels + a runner with docker
# CLI. Builds images, waits for healthy panels, runs the tagged Go test, and
# always tears down.
set -eu
cd "$(dirname "$0")"
compose='docker compose -f docker-compose.yml'
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then $compose ps || true; $compose logs --no-color || true; fi
  $compose down -v --remove-orphans || true
  exit "$status"
}
trap cleanup EXIT INT TERM
$compose build
$compose up -d --wait
go test -tags=easypanel_e2e . -count=1 -v
