# Heimdall

**Exposure intelligence platform for internet infrastructure analysis.**

*[Versión en español](README.es.md)* This repository provides the foundation: RIR dataset versioning, scope import pipeline, IP-to-country resolution, and country-level inventory APIs.

---

## What is Heimdall?

Heimdall aims to provide a structured, up-to-date view of global IP space as a foundation for asset discovery, internet scanning, and exposure intelligence workflows. The current phase focuses on **authoritative scope data**: downloading and versioning RIR (Regional Internet Registry) delegated statistics, importing IP and ASN blocks by country, and exposing APIs to resolve IPs to countries and to list blocks per country.

---

## What it does today

- **dataset-service** — Downloads and versions RIR delegated statistics files (`delegated-*-latest`). Validates headers, stores artifacts, and exposes them via API. Idempotent per registry/serial.
- **scope-service** — Imports blocks from dataset-service (by `dataset_id`), persists them, and provides:
  - **IP → country** resolution using a logical snapshot built from the latest imported dataset of each RIR (so the “current” view combines the newest RIPE, ARIN, APNIC, etc., without mixing stale data).
  - **Country inventory**: blocks and summary (IPv4/IPv6 counts) per country, with optional filtering by `dataset_id` or address family.
  - **ASN inventory**: list ASN ranges and summary (asn_range_count, asn_total_count) per country. ASN inventory reflects RIR delegated assignments, not current BGP origin AS relationships.
  - **Sync**: one-shot sync that fetches the latest validated dataset per registry from dataset-service and imports any missing ones.
- **Tests** — Unit and integration tests for parser, import pipeline, storage, and handlers (tests pass with Go and optional local Postgres).

---

## Architecture (current)

```
                    +------------------+
                    |   PostgreSQL     |
                    | (2 DBs: datasets |
                    |  + scope)        |
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
| **dataset-service** | Fetch, validate, version, and serve RIR datasets | 8080         |
| **scope-service**   | Import blocks, resolve IP→country, country inventory, sync | 8081    |
| **PostgreSQL**      | Two databases: `heimdall_datasets`, `heimdall_scope_service` | 5432   |

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

1. **Postgres** (port 5432), creating `heimdall_datasets` and `heimdall_scope_service` on first run.
2. **dataset-service** (port 8080), after Postgres is healthy.
3. **scope-service** (port 8081), after dataset-service is healthy.

Volumes: `postgres_data`, `dataset_artifacts` (downloaded RIR files). No `.env` required for this; everything is set in `docker-compose.yml`.

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

---

## Example endpoints

### dataset-service (port 8080)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/datasets/fetch?registry=ripencc\|arin\|apnic\|lacnic\|afrinic\|all` | Fetch and register a new version (idempotent by serial). |
| GET | `/v1/datasets` | List dataset versions (newest first). |
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

OpenAPI specs: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`.

---

## Local development (without Docker)

1. **Create databases:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   ```

2. **Environment:** Copy `configs/.env.example` to `.env` and set DSNs, ports, and `DATASET_SERVICE_URL` (see comments in the example).

3. **Run (two terminals):**

   ```bash
   # Terminal 1 – dataset-service (PORT=8080)
   go run ./cmd/dataset-service

   # Terminal 2 – scope-service (PORT=8081)
   go run ./cmd/scope-service
   ```

4. **Tests:**

   ```bash
   go test ./...
   ```

   Integration tests for storage and import assume Postgres is available (same DSN as in `.env` or defaults in test code). Skip is used if the DB is unreachable.

---

## Project status

- **Phase:** Development. Current focus is **RIR inventory and scope resolution** (datasets, import, by-IP, country blocks/summary, multi-RIR sync).
- **Not yet:** A full scanning engine or exposure pipeline; this repo is the data and API foundation for such a system.

---

## License

MIT. See [LICENSE](LICENSE).
