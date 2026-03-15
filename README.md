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
- **dataset-service** — Also supports CAIDA sources (`?source=caida_pfx2as_ipv4`, `caida_pfx2as_ipv6`, `caida_as_org`) for routing-service to consume.
- **Tests** — Unit and integration tests for parser, import pipeline, storage, and handlers (tests pass with Go and optional local Postgres).

---

## Architecture (current)

```
                    +------------------+
                    |   PostgreSQL     |
                    | (3 DBs: datasets|
                    |  scope, routing)|
                    +--------+---------+
                             |
         +-------------------+-------------------+
         |                                       |
  +------v------+                      +--------v--------+
  | dataset-    |   HTTP (artifact      | scope-service   |
  | service     |   + metadata)         | (import, query, |
  | (fetch,     |<---------------------|  by-ip, country)|
  |  version)   |                       +----------------+
  +-------------+
         ^
         | FTP/HTTP (RIR)
    RIPE NCC, ARIN, APNIC, LACNIC, AFRINIC
```

- **dataset-service**: Talks to RIR FTP/HTTP, parses RIR format, stores artifacts and metadata in Postgres (`heimdall_datasets`).
- **scope-service**: Pulls artifacts from dataset-service, parses and filters blocks (country, IPv4/IPv6), stores in Postgres (`heimdall_scope_service`). Resolves IPs and serves country inventory using a **logical snapshot**: the latest imported dataset from each registry (RIPE, ARIN, APNIC, etc.) is combined into one coherent view when no `dataset_id` is given.

---

## Services

| Service          | Role                                      | Default port |
|------------------|-------------------------------------------|--------------|
| **dataset-service** | Fetch, validate, version RIR and CAIDA datasets; serve artifacts | 8080         |
| **scope-service**   | **Assigned resource inventory.** Import blocks, IP→country, country/ASN inventory, sync | 8081    |
| **routing-service** | **Observed routing state.** Import pfx2as + as-org, IP→ASN (LPM), ASN metadata, ASN→prefixes | 8082    |
| **PostgreSQL**      | Databases: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service` | 5432   |

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

1. **Postgres** (port 5432), creating `heimdall_datasets`, `heimdall_scope_service`, and `heimdall_routing_service` on first run.
2. **dataset-service** (port 8080), after Postgres is healthy.
3. **scope-service** (port 8081) and **routing-service** (port 8082), after dataset-service is healthy.

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

---

## Heimdallctl (CLI)

**heimdallctl** is the official CLI for operating Heimdall. It talks to dataset-, scope-, and routing-service over HTTP (no business logic or DB in the CLI). Human-readable output by default; use `-o json` for scripting.

**Build:** (binary is not committed; use `bin/` or any path you prefer)

```bash
mkdir -p bin && go build -o bin/heimdallctl ./cmd/heimdallctl
```

**Configuration:** Environment variables override config files. Default base URLs: `http://localhost:8080` (dataset), `http://localhost:8081` (scope), `http://localhost:8082` (routing). Optional config file: `~/.config/heimdall/config.yaml` or `.heimdall.yaml` in the current directory. Set `HEIMDALL_DATASET_URL`, `HEIMDALL_SCOPE_URL`, `HEIMDALL_ROUTING_URL`, `HEIMDALL_TIMEOUT` (seconds) to override.

**Examples:**

```bash
# Health of all three services
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

# JSON output for scripting
heimdallctl status -o json
heimdallctl scope by-ip 8.8.8.8 -o json
```

See the service OpenAPI specs for full semantics of each operation.

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

OpenAPI specs: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`, `api/openapi/routing-service.yaml`.

---

## Local development (without Docker)

1. **Create databases:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   CREATE DATABASE heimdall_routing_service;
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
   ```

4. **Tests:**

   ```bash
   go test ./...
   ```

   Integration tests for storage and import assume Postgres is available (same DSN as in `.env` or defaults in test code). Skip is used if the DB is unreachable.

---

## Project status

- **Phase:** Development. **scope-service: assigned resource inventory.** **routing-service: observed routing state.** RIR + CAIDA datasets, scope resolution, country/ASN inventory, IP→ASN (LPM) and ASN metadata/prefixes.
- **Not yet:** A full scanning engine or exposure pipeline; this repo is the data and API foundation for such a system.

---

## License

MIT. See [LICENSE](LICENSE).
