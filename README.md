<div align="center" markdown="1">

➡️ [Installation](#-installation) | [Usage](#-usage) | [API Reference](#-api-reference) | [Español](README.es.md) ⬅️

# Heimdall

**Exposure intelligence for internet infrastructure.**

**Structured IP space, target materialization, distributed scan orchestration.**

![Go][badge-go] ![License MIT][badge-license] ![Docker][badge-docker]

</div>

---

# 🤔 What is this?

Heimdall is an **exposure intelligence platform** for security and network teams. It gives you a structured, up-to-date view of global IP space and runs internet-scale scan campaigns with pluggable workers.

- **Data layer:** Ingest RIR delegated statistics (RIPE, ARIN, APNIC, LACNIC, AFRINIC) and CAIDA (prefix-to-AS, AS-org). Resolve **IP→country** and **IP→ASN**, and keep country/ASN inventory.
- **Targets:** Define scopes (include/exclude by country, ASN, prefix, world). **Materialize** to immutable CIDR snapshots and diff between them.
- **Campaigns:** Create campaigns (target + scan profile), launch manually or on a schedule. Runs are dispatched to a queue.
- **Execution plane:** Workers pull jobs, run port discovery (**ZMap** or **Masscan**), and report observations. Scale by adding workers or raising concurrency. Results are stored per job.
- **Inventory:** execution-service forwards job completions to **inventory-service**; assets (e.g. hosts), exposures (e.g. open ports) and observations are stored for current state and history; diff between executions.

Everything is **API-first** and **containerized**. One CLI, **heimdallctl**, drives install, sync, and operations (executions, workers, requeue, cancel).

---

# 🛠️ Installation

## Docker (recommended)

From the project root:

```bash
cd deployments/docker
docker compose up --build -d
```

This starts PostgreSQL, Redis, dataset-, scope-, routing-, target-, campaign-, execution-, and inventory-service, plus one scan-worker.

## One-command setup (datasets + sync)

After the stack is up, run the official CLI to fetch all RIR + CAIDA datasets, wait for validation, and sync scope and routing:

```bash
# From repo root — heimdallctl via Docker
./scripts/heimdallctl.sh install
# or
make ctl install
```

Use `install --skip-wait` to skip waiting for datasets. Optional config: `HEIMDALL_*_URL` env vars or `~/.config/heimdall/config.yaml`.

## Local build (no Docker)

- **Go 1.22+**, **PostgreSQL 15+**, **Redis**
- Create DBs: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service`, `heimdall_campaign_service`, `heimdall_execution_service`, `heimdall_inventory_service`
- Copy `configs/.env.example` to `.env`, set DSNs and service URLs
- Run each service: `go run ./cmd/dataset-service`, `./cmd/scope-service`, etc., `./cmd/execution-service`, `./cmd/inventory-service`; optionally `./cmd/scan-worker`
- CLI: `go build -o bin/heimdallctl ./cmd/heimdallctl`

---

## ‼️ Important links

| Installation | Usage | API (OpenAPI) | Spanish |
| :----------: | :---: | :------------: | :-----: |
| [↑ Installation](#-installation) | [↑ Usage](#-usage) | [api/openapi/](api/openapi/) | [README.es.md](README.es.md) |

---

## 🙋 Table of contents

- [What is this?](#-what-is-this)
- [Installation](#-installation)
- [Features](#-features)
- [Why Heimdall?](#-why-heimdall)
- [Usage](#-usage)
- [heimdallctl](#-heimdallctl-cli)
- [API reference](#-api-reference)
- [Services](#-services)
- [Scan workers](#-scan-workers)
- [Local development](#-local-development)
- [License](#-license)

---

# ✨ Features

- **Dataset pipeline** — Fetch and version RIR and CAIDA datasets. Idempotent, validated, artifact storage.
- **Scope service** — Assigned inventory: IP→country, country/ASN blocks and summary, sync from dataset-service.
- **Routing service** — Observed routing: CAIDA import, IP→ASN (LPM), ASN metadata, ASN→prefixes.
- **Target service** — Define targets (country, ASN, prefix, world). Materialize to CIDR snapshots; diff between snapshots.
- **Campaign service** — Control plane: campaigns, scan profiles, runs. Manual or scheduled launch; dispatch to Redis Streams.
- **Execution service** — Execution plane: consume runs, create executions and jobs; worker register, heartbeat, claim, complete/fail/renew, **requeue** and **cancel**.
- **Inventory service** — Source of truth for assets, exposures and observations; ingest from execution-service on job complete; current state (first_seen/last_seen) and immutable history; diff between two executions.
- **Scan workers** — Pull-based; ZMap or Masscan; horizontal scale and per-worker concurrency.
- **heimdallctl** — Status, **install** (datasets + sync), dataset/scope/routing/target/campaign, **execution** and **worker** (list, jobs, requeue, cancel, update concurrency).

---

# 🔭 Why Heimdall?

## Data you can trust

Scope comes from **RIR delegated statistics**; routing from **CAIDA** (BGP-derived). You get IP→country and IP→ASN with clear semantics (assigned vs observed). Targets materialize to concrete CIDR sets you can diff and audit.

## Scale out

Workers **pull** jobs and renew leases while they work. Add more containers or raise concurrency; the execution-service assigns jobs with `SKIP LOCKED`. No single bottleneck.

## One CLI

**heimdallctl** talks to every service over HTTP. Run `install` once to bootstrap datasets and sync; then use `execution list`, `execution jobs`, `worker list`, `worker update --max-concurrency`, `execution requeue` / `execution cancel` without touching the DB.

---

# 🤸 Usage

1. **Install** — `heimdallctl install` (or fetch RIR/CAIDA and sync scope + routing manually).
2. **Target** — Create a target (e.g. country ES), materialize to get a CIDR snapshot.
3. **Campaign** — Create a scan profile (e.g. portscan-basic), create a campaign (target + profile, schedule manual), launch.
4. **Execution** — execution-service consumes the run from Redis, creates an execution and jobs; scan-workers claim jobs, run ZMap/Masscan, report observations in `result_summary`.

Quick curl flow (replace `<ids>`):

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

# Execution / workers
curl "http://localhost:8085/v1/executions?limit=10"
curl "http://localhost:8085/v1/executions/<execution_id>/jobs?limit=20"
curl -X POST "http://localhost:8085/v1/executions/<execution_id>/requeue"
curl -X POST "http://localhost:8085/v1/executions/<execution_id>/cancel"
curl "http://localhost:8085/v1/workers"

# Inventory (assets, exposures, observations; diff between executions)
curl "http://localhost:8086/v1/assets?limit=10"
curl "http://localhost:8086/v1/observations?execution_id=<execution_id>&limit=10"
curl "http://localhost:8086/v1/diffs/executions?from_execution_id=<id>&to_execution_id=<id>"
```

---

# 🎛️ heimdallctl (CLI)

Run via Docker: `./scripts/heimdallctl.sh <cmd>` or `make ctl <cmd>`. Or build: `go build -o bin/heimdallctl ./cmd/heimdallctl`.

| Area | Commands |
|------|----------|
| **Status** | `status` |
| **Install** | `install` — fetch RIR + CAIDA, wait validated, sync scope & routing; `install --skip-wait` |
| **Dataset** | `dataset fetch --registry=all \| ripencc \| ...`, `dataset fetch --source=caida_pfx2as_ipv4 \| ...`, `dataset list`, `dataset get <id>` |
| **Scope** | `scope sync`, `scope by-ip <ip>`, `scope country summary \| blocks \| asns \| asn-summary <cc>` |
| **Routing** | `routing sync`, `routing by-ip <ip>`, `routing asn <asn>`, `routing asn prefixes <asn>` |
| **Target** | `target list \| create \| get \| update \| materialize \| materializations \| prefixes \| diff` |
| **Campaign** | `campaign list \| create \| get \| launch \| runs`, `campaign run get \| cancel`, `scan-profile list \| create \| get` |
| **Execution** | `execution list \| get <id> \| jobs <id> \| requeue <id> \| cancel <id>` |
| **Worker** | `worker list \| get <id> \| jobs [worker-id] \| update <id> --max-concurrency N` |

Use `-o json` for machine-readable output.

---

# 📡 API reference

OpenAPI specs (request/response schemas and semantics):

| Service | Spec |
|--------|------|
| dataset | [api/openapi/dataset-service.yaml](api/openapi/dataset-service.yaml) |
| scope | [api/openapi/scope-service.yaml](api/openapi/scope-service.yaml) |
| routing | [api/openapi/routing-service.yaml](api/openapi/routing-service.yaml) |
| target | [api/openapi/target-service.yaml](api/openapi/target-service.yaml) |
| campaign | [api/openapi/campaign-service.yaml](api/openapi/campaign-service.yaml) |
| execution | [api/openapi/execution-service.yaml](api/openapi/execution-service.yaml) |
| inventory | [api/openapi/inventory-service.yaml](api/openapi/inventory-service.yaml) |

Execution-service: workers (list, register, get, PATCH heartbeat/max_concurrency, list jobs), jobs (claim, complete, fail, renew), executions (list, get, list jobs, **requeue**, **cancel**). On job complete it notifies inventory-service (fire-and-forget). Inventory-service: ingest (POST job-completed), assets, exposures, observations (with filters), diff between two executions.

---

# 📦 Services

| Service | Purpose | Port |
|--------|---------|------|
| **dataset-service** | Fetch, validate, version RIR and CAIDA; serve artifacts | 8080 |
| **scope-service** | Assigned inventory: import, IP→country, country/ASN | 8081 |
| **routing-service** | Observed routing: pfx2as, as-org, IP→ASN, ASN metadata/prefixes | 8082 |
| **target-service** | Targets, materialization, snapshots, diff | 8083 |
| **campaign-service** | Campaigns, scan profiles, runs; dispatch to Redis | 8084 |
| **execution-service** | Executions, jobs, workers (claim, complete, fail, requeue, cancel) | 8085 |
| **inventory-service** | Assets, exposures, observations; ingest from execution-service; diff executions | 8086 |
| **Redis** | Stream `heimdall:campaign:runs` | 6379 |
| **PostgreSQL** | All service DBs | 5432 |

---

# 🔧 Scan workers

Workers register with execution-service, heartbeat, and **pull** jobs. Each job has prefixes and port config; the worker runs **ZMap** or **Masscan** (`SCAN_ENGINE=zmap|masscan`, default masscan), parses results into observations (IP, port, status), and sends them in `result_summary`. Observations are stored in `execution_jobs.result_summary`. On job complete, execution-service notifies inventory-service (best-effort); inventory-service upserts assets/exposures and appends observations.

**Scale:** more replicas (`docker compose up -d --scale scan-worker=5`) and/or higher `WORKER_MAX_CONCURRENCY` per worker.

---

# 🧪 Local development

1. Create all Postgres DBs (see [Installation](#-installation)).
2. Copy `configs/.env.example` to `.env`, set DSNs and service URLs.
3. Run services in separate terminals: `go run ./cmd/dataset-service`, `./cmd/scope-service`, `./cmd/routing-service`, `./cmd/target-service`, `./cmd/campaign-service`, `./cmd/execution-service`, `./cmd/inventory-service`; optionally `./cmd/scan-worker`.
4. Tests: `go test ./...`

---

# 📄 License

MIT. See [LICENSE](LICENSE).

---

<!-- Badges -->

[badge-go]: https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go
[badge-license]: https://img.shields.io/badge/License-MIT-green?style=flat
[badge-docker]: https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker
