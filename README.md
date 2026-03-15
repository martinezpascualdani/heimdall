# Heimdall — Exposure Intelligence Platform

**Internet infrastructure analysis, asset discovery, and scanning orchestration.** Heimdall provides a structured, up-to-date view of global IP space: RIR and CAIDA dataset versioning, scope and routing APIs, target materialization, campaign control, and a distributed execution plane with scalable scan workers.

*[Versión en español](README.es.md)*

---

## Overview

Heimdall is an exposure intelligence platform for security and network teams. It ingests authoritative and observed data (RIR delegated statistics, CAIDA prefix-to-AS and AS-org), resolves IP-to-country and IP-to-ASN, materializes targets (e.g. by country or ASN), and runs scan campaigns via a control plane (campaign-service) and execution plane (execution-service + scan-workers). Workers pull jobs, run port discovery (ZMap or Masscan), and report results. The system is API-first, containerized, and designed to scale.

---

## Features

- **Dataset pipeline** — Fetch and version RIR delegated stats (RIPE, ARIN, APNIC, LACNIC, AFRINIC) and CAIDA (pfx2as IPv4/IPv6, as-org). Idempotent, artifact storage, validation.
- **Scope service** — Assigned resource inventory: import blocks by registry, IP→country resolution, country and ASN inventory (blocks, summary).
- **Routing service** — Observed routing state: CAIDA import, IP→ASN (longest-prefix match), ASN metadata, ASN→prefixes.
- **Target service** — Define targets (include/exclude by country, ASN, prefix, world). Materialize to immutable CIDR snapshots; diff between snapshots.
- **Campaign service** — Control plane: campaigns (target + scan profile), scan profiles, runs. Manual or scheduled launch; dispatch to Redis Streams.
- **Execution service** — Execution plane: consume runs from Redis, create executions and jobs, worker registration, heartbeat, job claim/complete/fail/renew, requeue and cancel.
- **Scan workers** — Pull-based agents: register, claim jobs, run port discovery (ZMap or Masscan), report observations. Scale horizontally; multiple workers and concurrency per worker supported.
- **heimdallctl** — Official CLI: status, dataset fetch/list, scope and routing sync, target and campaign management, **install** (full dataset + sync), **execution** and **worker** management (list, get, jobs, requeue, cancel, update concurrency).

---

## Services

| Service | Purpose | Port |
|--------|---------|------|
| **dataset-service** | Fetch, validate, version RIR and CAIDA datasets; serve artifacts | 8080 |
| **scope-service** | Assigned inventory: import blocks, IP→country, country/ASN inventory, sync | 8081 |
| **routing-service** | Observed routing: import pfx2as + as-org, IP→ASN (LPM), ASN metadata/prefixes | 8082 |
| **target-service** | Target definitions, materialization to CIDR snapshots, diff | 8083 |
| **campaign-service** | Control plane: campaigns, scan profiles, runs; dispatch to Redis | 8084 |
| **execution-service** | Execution plane: runs→executions/jobs, worker API (claim, complete, fail, requeue, cancel) | 8085 |
| **Redis** | Stream `heimdall:campaign:runs` (campaign→execution) | 6379 |
| **PostgreSQL** | Datasets, scope, routing, target, campaign, execution DBs | 5432 |

---

## Requirements

- **Go 1.22+** (local build and tests)
- **PostgreSQL 15+**
- **Docker & Docker Compose** (recommended)

---

## Quick start (Docker)

From the project root:

```bash
cd deployments/docker
docker compose up --build -d
```

This starts Postgres (with all DBs), dataset-, scope-, routing-, target-, campaign-, and execution-service, Redis, and (by default) one scan-worker. Volumes: `postgres_data`, `dataset_artifacts`.

**One-command setup (datasets + scope/routing sync):**

```bash
# Using heimdallctl via Docker (from repo root)
docker compose -f deployments/docker/docker-compose.yml --profile cli run --rm heimdallctl install
# Or with the wrapper:
./scripts/heimdallctl.sh install
# Or: make ctl install
```

`heimdallctl install` fetches RIR and CAIDA datasets, waits for them to be validated, then syncs scope and routing in parallel. Use `--skip-wait` to skip the wait step.

---

## Basic usage flow

1. **Datasets** — Fetch RIR and/or CAIDA, then sync scope and routing (or use `heimdallctl install` once).
2. **Target** — Create a target (e.g. country ES), materialize to get a CIDR snapshot.
3. **Campaign** — Create a scan profile (e.g. portscan-basic), create a campaign (target + profile, schedule manual), launch.
4. **Execution** — execution-service consumes the run from Redis, creates an execution and jobs; scan-workers claim jobs, run ZMap/Masscan, report results. Observations are stored in job `result_summary`.

Example (curl; replace IDs as needed):

```bash
# Target
curl -X POST "http://localhost:8083/v1/targets" -H "Content-Type: application/json" \
  -d '{"name":"Spain","rules":[{"kind":"include","selector_type":"country","selector_value":"ES"}]}'
curl -X POST "http://localhost:8083/v1/targets/<target_id>/materialize"

# Campaign
curl -X POST "http://localhost:8084/v1/scan-profiles" -H "Content-Type: application/json" \
  -d '{"name":"Portscan basic","slug":"portscan-basic","config":{"ports":[21,22,80,443,3389]}}'
curl -X POST "http://localhost:8084/v1/campaigns" -H "Content-Type: application/json" \
  -d '{"name":"Spain portscan","target_id":"<target_id>","scan_profile_id":"<profile_id>","schedule_type":"manual","materialization_policy":"use_latest"}'
curl -X POST "http://localhost:8084/v1/campaigns/<campaign_id>/launch"

# Execution / workers (list executions, list jobs, requeue failed, cancel)
curl "http://localhost:8085/v1/executions?limit=10"
curl "http://localhost:8085/v1/executions/<execution_id>/jobs?limit=20"
curl -X POST "http://localhost:8085/v1/executions/<execution_id>/requeue"
curl -X POST "http://localhost:8085/v1/executions/<execution_id>/cancel"
curl "http://localhost:8085/v1/workers"
```

---

## heimdallctl (CLI)

Official CLI for operating Heimdall over HTTP. Build locally: `go build -o bin/heimdallctl ./cmd/heimdallctl`. Or run via Docker: `./scripts/heimdallctl.sh <cmd>` or `make ctl <cmd>`.

**Configuration:** Set `HEIMDALL_DATASET_URL`, `HEIMDALL_SCOPE_URL`, `HEIMDALL_ROUTING_URL`, `HEIMDALL_TARGET_URL`, `HEIMDALL_CAMPAIGN_URL`, `HEIMDALL_EXECUTION_URL` (defaults: localhost 8080–8085). Optional: `~/.config/heimdall/config.yaml` or `.heimdall.yaml`.

**Commands (summary):**

| Area | Commands |
|------|----------|
| **Status** | `status` — health of all services |
| **Install** | `install` — fetch RIR + CAIDA, wait for validated, sync scope & routing; `install --skip-wait` to skip wait |
| **Dataset** | `dataset fetch --registry=all \| ripencc \| ...`, `dataset fetch --source=caida_pfx2as_ipv4 \| ...`, `dataset list`, `dataset get <id>` |
| **Scope** | `scope sync`, `scope by-ip <ip>`, `scope country summary \| blocks \| asns \| asn-summary <cc>` |
| **Routing** | `routing sync`, `routing by-ip <ip>`, `routing asn <asn>`, `routing asn prefixes <asn>` |
| **Target** | `target list \| create \| get \| update \| materialize \| materializations \| prefixes \| diff` |
| **Campaign** | `campaign list \| create \| get \| launch \| runs`, `campaign run get \| cancel`, `scan-profile list \| create \| get` |
| **Execution** | `execution list \| get <id> \| jobs <id> \| requeue <id> \| cancel <id>` |
| **Worker** | `worker list \| get <id> \| jobs [worker-id] \| update <id> --max-concurrency N` |

Use `-o json` for machine-readable output.

---

## API reference (OpenAPI)

Full semantics and request/response schemas:

- `api/openapi/dataset-service.yaml`
- `api/openapi/scope-service.yaml`
- `api/openapi/routing-service.yaml`
- `api/openapi/target-service.yaml`
- `api/openapi/campaign-service.yaml`
- `api/openapi/execution-service.yaml`

Execution-service covers: workers (list, register, get, PATCH heartbeat/max_concurrency, list jobs), jobs (claim, complete, fail, renew), executions (list, get, list jobs, **requeue**, **cancel**).

---

## Local development (without Docker)

1. Create Postgres databases: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service`, `heimdall_campaign_service`, `heimdall_execution_service`.
2. Copy `configs/.env.example` to `.env` and set DSNs and service URLs.
3. Run services (each in its own terminal): `go run ./cmd/dataset-service`, `./cmd/scope-service`, `./cmd/routing-service`, `./cmd/target-service`, `./cmd/campaign-service`, `./cmd/execution-service`; optionally `./cmd/scan-worker`.
4. Tests: `go test ./...`

---

## Scan workers

Workers register with execution-service, send heartbeats, and claim jobs (pull). Each job contains prefixes and port/config; the worker runs **ZMap** or **Masscan** (configurable via `SCAN_ENGINE=zmap|masscan`, default masscan), parses results into observations (IP, port, status), and reports them in `result_summary`. Observations are stored in `execution_jobs.result_summary`. Scale by running more worker containers (`docker compose up -d --scale scan-worker=5`) and/or increasing `WORKER_MAX_CONCURRENCY` per worker.

---

## Project status

**Phase:** Development. Full pipeline from datasets to scope/routing, targets, campaigns, execution plane, and scan workers is implemented. Port discovery (ZMap, Masscan), job leasing, requeue, cancel, and heimdallctl install/execution/worker are in place. Future work may add more scan types, result aggregation APIs, and UI.

---

## License

MIT. See [LICENSE](LICENSE).
