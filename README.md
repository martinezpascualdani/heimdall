# Heimdall

**Exposure intelligence platform for internet infrastructure analysis.**

*[Versión en español](README.es.md)* This repository provides the foundation: RIR dataset versioning, scope import pipeline, IP-to-country resolution, and country-level inventory APIs.

---

## What is Heimdall?

Heimdall aims to provide a structured, up-to-date view of global IP space as a foundation for asset discovery, internet scanning, and exposure intelligence workflows. The current phase focuses on **authoritative scope data**: downloading and versioning RIR (Regional Internet Registry) delegated statistics, importing IP and ASN blocks by country, and exposing APIs to resolve IPs to countries and to list blocks per country.

---

## What it does today

- **dataset-service** — Downloads and versions RIR delegated statistics files (`delegated-*-latest`). Validates headers, stores artifacts, and exposes them via API. Idempotent per registry/serial.
- **scope-service** — **scope-service: assigned resource inventory.** Imports blocks from dataset-service (by `dataset_id`), persists them, and provides IP→country resolution, country inventory (blocks, summary, ASN ranges). Sync fetches latest validated per registry and imports. ASN inventory reflects RIR delegated assignments, not current BGP origin AS relationships.
- **routing-service** — **routing-service: observed routing state.** Imports CAIDA RouteViews Prefix-to-AS (pfx2as) and AS Organizations from dataset-service. Provides IP→ASN (longest-prefix match), ASN metadata (org/name), and ASN→prefixes. Data is BGP-derived, not assignation.
- **target-service** — Target definitions (include/exclude by country, ASN, prefix, world), materialization to CIDR snapshots, and diff between snapshots. Consumes scope-service and routing-service. Snapshots are immutable. **Exclusion by prefix (v1):** removes only entire materialized prefixes contained in the exclusion CIDR; does *not* perform CIDR subtraction or splitting.
- **campaign-service** — Control plane for execution: campaigns (target + scan profile, schedule), scan profiles, and runs. Dispatches run payloads to Redis Streams for future workers. Scheduling: manual, once, interval. `/ready` requires DB and Redis.
- **dataset-service** — Also supports CAIDA sources (`?source=caida_pfx2as_ipv4`, `caida_pfx2as_ipv6`, `caida_as_org`) for routing-service to consume.
- **Tests** — Unit and integration tests for parser, import pipeline, storage, and handlers (tests pass with Go and optional local Postgres).

---

## Architecture (current)

```
                         +------------------+
                         |   PostgreSQL     |
                         | (5 DBs: datasets,|
                         |  scope, routing, |
                         |  target, campaign)|
                         +--------+---------+
                                  |
     +-----------------------------+-----------------------------+------------------+
     |                             |                             |                  |
+----v----+   HTTP   +-------------v-------------+   HTTP   +-----v-----+   HTTP   +-v----------+
| dataset |<---------| scope-service             |<---------| target-   |<---------| routing-   |
| service |          | (import, by-ip, country)  |          | service   |          | service    |
| (fetch, |          +--------------------------+          | (define,  |          | (by-ip,    |
| version)|                                               |  material-|          |  asn,      |
+----^----+                                               |  ize,     |          |  prefixes) |
     |                                                     |  diff)    |          +------------+
     | FTP/HTTP (RIR)                                      +-----^-----+
RIPE NCC, ARIN, APNIC, LACNIC, AFRINIC                            |
                                                                   | HTTP
                                                            +------v------+     +-------+
                                                            | campaign-   |---->| Redis |
                                                            | service     |     |Streams|
                                                            | (campaigns, |     +---^---+
                                                            |  runs,      |         |
                                                            |  dispatch)  |     (workers)
                                                            +-------------+
heimdallctl (CLI)  ----HTTP---->  dataset  scope  routing  target  campaign
```

- **dataset-service**: Talks to RIR FTP/HTTP, parses RIR format, stores artifacts and metadata in Postgres (`heimdall_datasets`). Also fetches CAIDA datasets for routing.
- **scope-service**: Pulls artifacts from dataset-service, parses and filters blocks (country, IPv4/IPv6), stores in Postgres (`heimdall_scope_service`). Resolves IPs and serves country inventory using a **logical snapshot**: the latest imported dataset from each registry (RIPE, ARIN, APNIC, etc.) is combined into one coherent view when no `dataset_id` is given.
- **routing-service**: Pulls CAIDA (pfx2as, as-org) from dataset-service, stores in Postgres (`heimdall_routing_service`). Exposes IP→ASN (LPM), ASN metadata, ASN→prefixes.
- **target-service**: Stores target definitions (rules: include/exclude by country, ASN, prefix, world) in Postgres (`heimdall_target_service`). Materializes targets by calling scope-service and routing-service, persists immutable CIDR snapshots. Supports diff between snapshots. **World** = union of RIR inventory per country (operational view, not “all BGP”). **Exclusion by prefix:** v1 only removes entire materialized prefixes contained in the exclusion CIDR; no CIDR subtraction/splitting.
- **campaign-service**: Control plane: campaigns (target_id, scan_profile_id, schedule: manual/once/interval), scan profiles, and campaign runs. Persists in Postgres (`heimdall_campaign_service`). Resolves materialization via target-service (use_latest or rematerialize), publishes run payloads to Redis Streams (`heimdall:campaign:runs`). Scheduler (optional) creates runs for due campaigns. `/ready` requires DB and Redis.
- **heimdallctl**: Official CLI; queries dataset-, scope-, routing-, target-, and campaign-service over HTTP. No database or business logic; build with `go build -o bin/heimdallctl ./cmd/heimdallctl`.

---

## Services

| Service          | Role                                      | Default port |
|------------------|-------------------------------------------|--------------|
| **dataset-service** | Fetch, validate, version RIR and CAIDA datasets; serve artifacts | 8080         |
| **scope-service**   | **Assigned resource inventory.** Import blocks, IP→country, country/ASN inventory, sync | 8081    |
| **routing-service** | **Observed routing state.** Import pfx2as + as-org, IP→ASN (LPM), ASN metadata, ASN→prefixes | 8082    |
| **target-service**  | Target definitions, rules, materialization to CIDR sets, snapshots and diff | 8083    |
| **campaign-service** | Control plane: campaigns, scan profiles, runs; dispatch to Redis Streams | 8084    |
| **Redis**           | Stream `heimdall:campaign:runs` for run dispatch (workers consume) | 6379    |
| **PostgreSQL**      | Databases: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service`, `heimdall_campaign_service` | 5432   |

---

## Requirements

- **Go 1.22+** (for local build and tests)
- **PostgreSQL 15+** (for local run or via Docker)
- **Docker & Docker Compose** (recommended for running the stack)

---

## Quick start (Docker)

From the project root:

```bash
cd deployments/docker
docker compose up --build -d
```

This starts:

1. **Postgres** (port 5432), creating `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service`, and `heimdall_campaign_service` on first run.
2. **dataset-service** (port 8080), after Postgres is healthy.
3. **scope-service** (port 8081) and **routing-service** (port 8082), after dataset-service is healthy.
4. **target-service** (port 8083), after scope and routing are healthy.
5. **Redis** (port 6379) and **campaign-service** (port 8084), after target-service is healthy.

Volumes: `postgres_data`, `dataset_artifacts`. No `.env` required; everything is set in `docker-compose.yml`.

---

## Basic usage flow

1. **Fetch** one or more RIR datasets (e.g. RIPE, ARIN, APNIC):

   ```bash
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=ripencc"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=arin"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=all"
   ```

2. **Sync** scope-service with the latest validated dataset per registry (imports what’s missing):

   ```bash
   curl -X POST "http://localhost:8081/v1/imports/sync"
   ```

   Or import a specific dataset:

   ```bash
   curl -X POST "http://localhost:8081/v1/import?dataset_id=<uuid>"
   ```

3. **Resolve** an IP to country:

   ```bash
   curl "http://localhost:8081/v1/scopes/by-ip/8.8.8.8"
   ```

4. **Country inventory** (blocks and summary):

   ```bash
   curl "http://localhost:8081/v1/scopes/country/ES/summary"
   curl "http://localhost:8081/v1/scopes/country/ES/blocks?limit=10"
   curl "http://localhost:8081/v1/scopes/country/ES/asns?limit=10"
   curl "http://localhost:8081/v1/scopes/country/ES/asn-summary"
   ```

5. **Routing (observed state)** — Fetch CAIDA datasets, sync routing-service, then resolve IP→ASN and query ASN metadata/prefixes:

   ```bash
   curl -X POST "http://localhost:8080/v1/datasets/fetch?source=caida_pfx2as_ipv4"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?source=caida_pfx2as_ipv6"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?source=caida_as_org"
   curl -X POST "http://localhost:8082/v1/imports/sync"
   curl "http://localhost:8082/v1/asn/by-ip/8.8.8.8"
   curl "http://localhost:8082/v1/asn/15169"
   curl "http://localhost:8082/v1/asn/prefixes/15169?limit=10"
   ```

6. **Target service** — Create a target (e.g. one country), materialize it to a CIDR snapshot, list snapshots and prefixes:

   ```bash
   curl -X POST "http://localhost:8083/v1/targets" -H "Content-Type: application/json" -d '{"name":"Spain","rules":[{"kind":"include","selector_type":"country","selector_value":"ES"}]}'
   # Use the returned target id:
   curl -X POST "http://localhost:8083/v1/targets/<target_id>/materialize"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations?limit=10"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations/<mid>/prefixes?limit=10"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations/diff?from=<mid1>&to=<mid2>"
   ```

   **Note:** Exclusion by prefix (v1) removes only entire materialized prefixes that are contained in the exclusion CIDR; the service does *not* perform CIDR subtraction or splitting of larger prefixes.

7. **Campaign service** — Create a scan profile, create a campaign (target + profile, schedule), launch manually or let the scheduler create runs; runs are dispatched to Redis:

   ```bash
   curl -X POST "http://localhost:8084/v1/scan-profiles" -H "Content-Type: application/json" -d '{"name":"web-fast","slug":"web-fast"}'
   # Use returned scan_profile id and existing target id:
   curl -X POST "http://localhost:8084/v1/campaigns" -H "Content-Type: application/json" -d '{"name":"Spain scan","target_id":"<target_id>","scan_profile_id":"<profile_id>","schedule_type":"manual","materialization_policy":"use_latest"}'
   curl -X POST "http://localhost:8084/v1/campaigns/<campaign_id>/launch"
   curl "http://localhost:8084/v1/campaigns/<campaign_id>/runs?limit=10"
   curl "http://localhost:8084/ready"
   ```

---

## Heimdallctl (CLI)

**heimdallctl** is the official CLI for operating Heimdall. It talks to dataset-, scope-, routing-, target-, and campaign-service over HTTP (no business logic or DB in the CLI). Human-readable output by default; use `-o json` for scripting.

**Build:** (binary is not committed; use `bin/` or any path you prefer)

```bash
mkdir -p bin && go build -o bin/heimdallctl ./cmd/heimdallctl
```

**Configuration:** Environment variables override config files. Default base URLs: `http://localhost:8080` (dataset), `http://localhost:8081` (scope), `http://localhost:8082` (routing), `http://localhost:8083` (target), `http://localhost:8084` (campaign). Optional config file: `~/.config/heimdall/config.yaml` or `.heimdall.yaml` in the current directory. Set `HEIMDALL_DATASET_URL`, `HEIMDALL_SCOPE_URL`, `HEIMDALL_ROUTING_URL`, `HEIMDALL_TARGET_URL`, `HEIMDALL_CAMPAIGN_URL`, `HEIMDALL_TIMEOUT` (seconds) to override.

**Examples:**

```bash
# Health of all services (dataset, scope, routing, target, campaign)
heimdallctl status

# Dataset: fetch (RIR or CAIDA), list, get
heimdallctl dataset fetch --registry=all
heimdallctl dataset fetch --source=caida_pfx2as_ipv4
heimdallctl dataset list
heimdallctl dataset get <uuid>

# Scope: sync, resolve IP, country summary/blocks/asns
heimdallctl scope sync
heimdallctl scope by-ip 8.8.8.8
heimdallctl scope country summary ES
heimdallctl scope country blocks ES --limit=10
heimdallctl scope country asns ES
heimdallctl scope country asn-summary ES
heimdallctl scope country datasets ES

# Routing: sync, IP→ASN, ASN metadata, ASN prefixes
heimdallctl routing sync
heimdallctl routing by-ip 8.8.8.8
heimdallctl routing asn 15169
heimdallctl routing asn prefixes 15169 --limit=10

# Target: create, list, get, update, materialize, diff
heimdallctl target list
heimdallctl target create --name Spain --rule include,country,ES
heimdallctl target get <target-id>
heimdallctl target update <target-id> --rule include,country,ES --rule include,country,PT
heimdallctl target materialize <target-id>
heimdallctl target materializations <target-id>
heimdallctl target prefixes <target-id> <materialization-id>
heimdallctl target diff <target-id> --from <mid1> --to <mid2>

# Campaign: list, create, launch, runs; scan-profile list/create; run get/cancel
heimdallctl campaign list
heimdallctl campaign create --name "Spain scan" --target-id <tid> --scan-profile-id <pid> --schedule-type manual --materialization-policy use_latest
heimdallctl campaign get <campaign-id>
heimdallctl campaign launch <campaign-id>
heimdallctl campaign runs <campaign-id>
heimdallctl campaign run get <run-id>
heimdallctl campaign run cancel <run-id>
heimdallctl scan-profile list
heimdallctl scan-profile create --name "web-fast" --slug web-fast
heimdallctl scan-profile get <profile-id>

# JSON output for scripting
heimdallctl status -o json
heimdallctl scope by-ip 8.8.8.8 -o json
```

See the service OpenAPI specs (`api/openapi/*.yaml`) for full semantics of each operation.

---

## Example endpoints

### dataset-service (port 8080)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/datasets/fetch?registry=...` or `?source=caida_pfx2as_ipv4\|caida_pfx2as_ipv6\|caida_as_org` | Fetch RIR or CAIDA dataset (idempotent). |
| GET | `/v1/datasets?source=&source_type=` | List dataset versions (optional filters: source, source_type rir\|caida). |
| GET | `/v1/datasets/{id}` | Metadata for one version. |
| GET | `/v1/datasets/{id}/artifact` | Stream artifact content. |
| GET | `/health`, `/ready`, `/version` | Health and version. |

### scope-service (port 8081)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/import?dataset_id=<uuid>` | Import a dataset into scope (idempotent). |
| POST | `/v1/imports/sync` | Sync: fetch latest per registry from dataset-service and import missing. |
| GET | `/v1/scopes/by-ip/{ip}` | Resolve IP to country (optional `?dataset_id=`). |
| GET | `/v1/scopes/country/{cc}/blocks` | Blocks for country (optional `dataset_id`, `address_family`, `limit`, `offset`). |
| GET | `/v1/scopes/country/{cc}/summary` | IPv4/IPv6/total counts for country. |
| GET | `/v1/scopes/country/{cc}/asns` | ASN ranges for country (RIR delegated; not BGP/IP→ASN). |
| GET | `/v1/scopes/country/{cc}/asn-summary` | asn_range_count and asn_total_count for country. |
| GET | `/v1/scopes/country/{cc}/datasets` | Imported datasets that have blocks for that country. |
| GET | `/health`, `/ready`, `/version` | Health and version. |

### routing-service (port 8082)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/imports/sync` | Sync: fetch latest CAIDA (pfx2as IPv4/IPv6, as-org) from dataset-service and import. |
| GET | `/v1/asn/by-ip/{ip}` | IP→ASN (longest-prefix match); optional `?dataset_id=` (routing snapshot only). |
| GET | `/v1/asn/{asn}` | ASN metadata (as_name, org_name); 404 if none. |
| GET | `/v1/asn/prefixes/{asn}` | Prefixes where primary_asn = ASN (deterministic order); `limit`, `offset`, `dataset_id`. |
| GET | `/health`, `/ready`, `/version` | Health and version. |

### target-service (port 8083)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/targets` | Create target (name, description, rules). |
| GET | `/v1/targets` | List targets (default active only; `?include_inactive=true`, `limit`, `offset`). |
| GET | `/v1/targets/{id}` | Get target with rules. |
| PUT | `/v1/targets/{id}` | Full replacement of definition. |
| DELETE | `/v1/targets/{id}` | Soft delete (idempotent). |
| POST | `/v1/targets/{id}/materialize` | Run materialization (v1 synchronous). |
| GET | `/v1/targets/{id}/materializations` | List snapshots (paginated). |
| GET | `/v1/targets/{id}/materializations/{mid}` | Snapshot metadata (no prefixes). |
| GET | `/v1/targets/{id}/materializations/{mid}/prefixes` | Prefixes (paginated). |
| GET | `/v1/targets/{id}/materializations/diff?from=&to=` | Diff between two snapshots (same target). |
| GET | `/health`, `/ready`, `/version` | Health and version. |

### campaign-service (port 8084)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/scan-profiles` | Create scan profile (name, slug). |
| GET | `/v1/scan-profiles` | List scan profiles (limit, offset). |
| GET | `/v1/scan-profiles/{id}` | Get scan profile. |
| PUT | `/v1/scan-profiles/{id}` | Update scan profile (full replacement). |
| DELETE | `/v1/scan-profiles/{id}` | Delete (rejected if campaigns use it). |
| POST | `/v1/campaigns` | Create campaign (target_id, scan_profile_id, schedule_type, materialization_policy). |
| GET | `/v1/campaigns` | List campaigns (include_inactive, limit, offset). |
| GET | `/v1/campaigns/{id}` | Get campaign. |
| PUT | `/v1/campaigns/{id}` | Update campaign (full replacement). |
| DELETE | `/v1/campaigns/{id}` | Soft delete. |
| POST | `/v1/campaigns/{id}/launch` | Launch campaign (create run, dispatch to Redis). |
| GET | `/v1/campaigns/{id}/runs` | List runs (limit, offset). |
| GET | `/v1/runs/{id}` | Get run. |
| POST | `/v1/runs/{id}/cancel` | Cancel run. |
| GET | `/health`, `/ready`, `/version` | Health, readiness (DB+Redis), version. |

OpenAPI specs: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`, `api/openapi/routing-service.yaml`, `api/openapi/target-service.yaml`, `api/openapi/campaign-service.yaml`.

---

## Local development (without Docker)

1. **Create databases:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   CREATE DATABASE heimdall_routing_service;
   CREATE DATABASE heimdall_target_service;
   CREATE DATABASE heimdall_campaign_service;
   ```

2. **Environment:** Copy `configs/.env.example` to `.env` and set DSNs, ports, and `DATASET_SERVICE_URL` (see comments in the example).

3. **Run (two terminals):**

   ```bash
   # Terminal 1 – dataset-service (PORT=8080)
   go run ./cmd/dataset-service

   # Terminal 2 – scope-service (PORT=8081)
   go run ./cmd/scope-service

   # Terminal 3 – routing-service (PORT=8082)
   go run ./cmd/routing-service

   # Terminal 4 – target-service (PORT=8083; needs scope + routing)
   go run ./cmd/target-service

   # Terminal 5 – campaign-service (PORT=8084; needs target-service, Postgres, Redis)
   go run ./cmd/campaign-service
   ```

4. **Tests:**

   ```bash
   go test ./...
   ```

   Integration tests for storage and import use **separate test databases** (`heimdall_*_test`) by default so they never touch development data. These are created by the same `init-db.sql` when using Docker. If Postgres or the test DBs are unavailable, tests skip. You can override with `DATASET_DB_DSN`, `SCOPE_DB_DSN`, `TARGET_DB_DSN`, etc. to point to your test instance.

---

## Project status

- **Phase:** Development. **scope-service: assigned resource inventory.** **routing-service: observed routing state.** **target-service:** target definitions, materialization to CIDR snapshots, diff. RIR + CAIDA datasets, scope resolution, country/ASN inventory, IP→ASN (LPM) and ASN metadata/prefixes.
- **Not yet:** A full scanning engine or exposure pipeline; this repo is the data and API foundation for such a system.

---

## License

MIT. See [LICENSE](LICENSE).
