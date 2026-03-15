#!/usr/bin/env bash
# Run heimdallctl via Docker (no Go on host). From repo root: ./scripts/heimdallctl.sh status | install | execution list | ...
# To clean orphan run containers: docker compose -f deployments/docker/docker-compose.yml --profile cli down --remove-orphans

set -e
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
exec docker compose -f deployments/docker/docker-compose.yml --profile cli run --rm heimdallctl "$@"
