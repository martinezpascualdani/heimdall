<div align="center" markdown="1">

➡️ [Instalación](#-instalación) | [Uso](#-uso) | [Referencia API](#-referencia-api) | [English](README.md) ⬅️

# Heimdall

**Exposure intelligence para infraestructura de Internet.**

**Espacio IP estructurado, materialización de targets, orquestación de escaneo distribuida.**

![Go][badge-go] ![Licencia MIT][badge-license] ![Docker][badge-docker]

</div>

---

# 🤔 ¿Qué es esto?

Heimdall es una **plataforma de exposure intelligence** para equipos de seguridad y red. Ofrece una vista estructurada y actualizada del espacio IP global y ejecuta campañas de escaneo a escala con workers escalables.

- **Capa de datos:** Ingesta de estadísticas delegadas RIR (RIPE, ARIN, APNIC, LACNIC, AFRINIC) y CAIDA (prefix-to-AS, AS-org). Resolución **IP→país** e **IP→ASN**, e inventario por país/ASN.
- **Targets:** Definición de alcances (include/exclude por país, ASN, prefijo, world). **Materialización** a snapshots CIDR inmutables y diff entre ellos.
- **Campañas:** Creación de campañas (target + perfil de escaneo), lanzamiento manual o programado. Los runs se despachan a una cola.
- **Plano de ejecución:** Los workers reclaman jobs por pull, ejecutan descubrimiento de puertos (**ZMap** o **Masscan**) y reportan observaciones. Escala añadiendo workers o subiendo concurrencia. Los resultados se guardan por job.

Todo es **API-first** y **containerizado**. Un único CLI, **heimdallctl**, cubre install, sync y operaciones (executions, workers, requeue, cancel).

---

# 🛠️ Instalación

## Docker (recomendado)

Desde la raíz del proyecto:

```bash
cd deployments/docker
docker compose up --build -d
```

Se levantan PostgreSQL, Redis, dataset-, scope-, routing-, target-, campaign- y execution-service, más un scan-worker.

## Setup en un comando (datasets + sync)

Con el stack arriba, ejecuta el CLI oficial para descargar todos los datasets RIR + CAIDA, esperar validación y sincronizar scope y routing:

```bash
# Desde la raíz del repo — heimdallctl vía Docker
./scripts/heimdallctl.sh install
# o
make ctl install
```

Usa `install --skip-wait` para no esperar a los datasets. Config opcional: variables `HEIMDALL_*_URL` o `~/.config/heimdall/config.yaml`.

## Build local (sin Docker)

- **Go 1.22+**, **PostgreSQL 15+**, **Redis**
- Crear BDs: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service`, `heimdall_campaign_service`, `heimdall_execution_service`
- Copiar `configs/.env.example` a `.env`, configurar DSNs y URLs de servicios
- Arrancar cada servicio: `go run ./cmd/dataset-service`, `./cmd/scope-service`, etc.; opcionalmente `./cmd/scan-worker`
- CLI: `go build -o bin/heimdallctl ./cmd/heimdallctl`

---

## ‼️ Enlaces importantes

| Instalación | Uso | API (OpenAPI) | English |
| :---------: | :-: | :-----------: | :-----: |
| [↑ Instalación](#-instalación) | [↑ Uso](#-uso) | [api/openapi/](api/openapi/) | [README.md](README.md) |

---

## 🙋 Índice

- [¿Qué es esto?](#-qué-es-esto)
- [Instalación](#-instalación)
- [Características](#-características)
- [Por qué Heimdall](#-por-qué-heimdall)
- [Uso](#-uso)
- [heimdallctl](#-heimdallctl-cli)
- [Referencia API](#-referencia-api)
- [Servicios](#-servicios)
- [Scan workers](#-scan-workers)
- [Desarrollo local](#-desarrollo-local)
- [Licencia](#-licencia)

---

# ✨ Características

- **Pipeline de datasets** — Descarga y versionado de datasets RIR y CAIDA. Idempotente, validado, almacenamiento de artefactos.
- **Scope service** — Inventario asignado: IP→país, bloques y resumen por país/ASN, sync desde dataset-service.
- **Routing service** — Routing observado: import CAIDA, IP→ASN (LPM), metadata ASN, ASN→prefijos.
- **Target service** — Definición de targets (país, ASN, prefijo, world). Materialización a snapshots CIDR; diff entre snapshots.
- **Campaign service** — Plan de control: campañas, perfiles de escaneo, runs. Lanzamiento manual o programado; dispatch a Redis Streams.
- **Execution service** — Plan de ejecución: consumir runs, crear executions y jobs; registro de workers, heartbeat, claim, complete/fail/renew, **requeue** y **cancel**.
- **Scan workers** — Pull-based; ZMap o Masscan; escala horizontal y concurrencia por worker.
- **heimdallctl** — Status, **install** (datasets + sync), dataset/scope/routing/target/campaign, **execution** y **worker** (list, jobs, requeue, cancel, update concurrency).

---

# 🔭 Por qué Heimdall

## Datos en los que confiar

El scope viene de **estadísticas delegadas RIR**; el routing de **CAIDA** (derivado de BGP). Obtienes IP→país e IP→ASN con semántica clara (asignado vs observado). Los targets se materializan en conjuntos CIDR concretos que puedes diffear y auditar.

## Escala horizontal

Los workers **reclaman** jobs por pull y renuevan el lease mientras trabajan. Añade más contenedores o sube la concurrencia; el execution-service asigna jobs con `SKIP LOCKED`. Sin cuello de botella único.

## Un solo CLI

**heimdallctl** habla con todos los servicios por HTTP. Ejecuta `install` una vez para arrancar datasets y sync; luego usa `execution list`, `execution jobs`, `worker list`, `worker update --max-concurrency`, `execution requeue` / `execution cancel` sin tocar la base de datos.

---

# 🤸 Uso

1. **Instalar** — `heimdallctl install` (o descargar RIR/CAIDA y sincronizar scope + routing a mano).
2. **Target** — Crear un target (ej. país ES), materializar para obtener un snapshot CIDR.
3. **Campaña** — Crear perfil de escaneo (ej. portscan-basic), crear campaña (target + perfil, schedule manual), lanzar.
4. **Ejecución** — execution-service consume el run desde Redis, crea execution y jobs; los scan-workers reclaman jobs, ejecutan ZMap/Masscan, reportan observaciones en `result_summary`.

Flujo rápido con curl (sustituir `<ids>`):

```bash
# Target
curl -X POST "http://localhost:8083/v1/targets" -H "Content-Type: application/json" \
  -d '{"name":"Spain","rules":[{"kind":"include","selector_type":"country","selector_value":"ES"}]}'
curl -X POST "http://localhost:8083/v1/targets/<target_id>/materialize"

# Campaña
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
```

---

# 🎛️ heimdallctl (CLI)

Ejecutar vía Docker: `./scripts/heimdallctl.sh <cmd>` o `make ctl <cmd>`. O compilar: `go build -o bin/heimdallctl ./cmd/heimdallctl`.

| Área | Comandos |
|------|----------|
| **Status** | `status` |
| **Install** | `install` — fetch RIR + CAIDA, esperar validados, sync scope y routing; `install --skip-wait` |
| **Dataset** | `dataset fetch --registry=all \| ripencc \| ...`, `dataset fetch --source=caida_pfx2as_ipv4 \| ...`, `dataset list`, `dataset get <id>` |
| **Scope** | `scope sync`, `scope by-ip <ip>`, `scope country summary \| blocks \| asns \| asn-summary <cc>` |
| **Routing** | `routing sync`, `routing by-ip <ip>`, `routing asn <asn>`, `routing asn prefixes <asn>` |
| **Target** | `target list \| create \| get \| update \| materialize \| materializations \| prefixes \| diff` |
| **Campaign** | `campaign list \| create \| get \| launch \| runs`, `campaign run get \| cancel`, `scan-profile list \| create \| get` |
| **Execution** | `execution list \| get <id> \| jobs <id> \| requeue <id> \| cancel <id>` |
| **Worker** | `worker list \| get <id> \| jobs [worker-id] \| update <id> --max-concurrency N` |

Usar `-o json` para salida machine-readable.

---

# 📡 Referencia API

Especificaciones OpenAPI (esquemas y semántica de peticiones/respuestas):

| Servicio | Spec |
|----------|------|
| dataset | [api/openapi/dataset-service.yaml](api/openapi/dataset-service.yaml) |
| scope | [api/openapi/scope-service.yaml](api/openapi/scope-service.yaml) |
| routing | [api/openapi/routing-service.yaml](api/openapi/routing-service.yaml) |
| target | [api/openapi/target-service.yaml](api/openapi/target-service.yaml) |
| campaign | [api/openapi/campaign-service.yaml](api/openapi/campaign-service.yaml) |
| execution | [api/openapi/execution-service.yaml](api/openapi/execution-service.yaml) |

Execution-service: workers (list, register, get, PATCH heartbeat/max_concurrency, list jobs), jobs (claim, complete, fail, renew), executions (list, get, list jobs, **requeue**, **cancel**).

---

# 📦 Servicios

| Servicio | Función | Puerto |
|----------|---------|--------|
| **dataset-service** | Fetch, validar, versionar RIR y CAIDA; servir artefactos | 8080 |
| **scope-service** | Inventario asignado: import, IP→país, país/ASN | 8081 |
| **routing-service** | Routing observado: pfx2as, as-org, IP→ASN, metadata/prefijos ASN | 8082 |
| **target-service** | Targets, materialización, snapshots, diff | 8083 |
| **campaign-service** | Campañas, perfiles de escaneo, runs; dispatch a Redis | 8084 |
| **execution-service** | Executions, jobs, workers (claim, complete, fail, requeue, cancel) | 8085 |
| **Redis** | Stream `heimdall:campaign:runs` | 6379 |
| **PostgreSQL** | Todas las BDs de servicios | 5432 |

---

# 🔧 Scan workers

Los workers se registran en execution-service, envían heartbeat y **reclaman** jobs por pull. Cada job tiene prefijos y configuración de puertos; el worker ejecuta **ZMap** o **Masscan** (`SCAN_ENGINE=zmap|masscan`, por defecto masscan), parsea resultados en observaciones (IP, puerto, estado) y las envía en `result_summary`. Las observaciones se guardan en `execution_jobs.result_summary`.

**Escalar:** más réplicas (`docker compose up -d --scale scan-worker=5`) y/o mayor `WORKER_MAX_CONCURRENCY` por worker.

---

# 🧪 Desarrollo local

1. Crear todas las BDs en Postgres (ver [Instalación](#-instalación)).
2. Copiar `configs/.env.example` a `.env`, configurar DSNs y URLs de servicios.
3. Arrancar servicios en terminales separadas: `go run ./cmd/dataset-service`, `./cmd/scope-service`, `./cmd/routing-service`, `./cmd/target-service`, `./cmd/campaign-service`, `./cmd/execution-service`; opcionalmente `./cmd/scan-worker`.
4. Tests: `go test ./...`

---

# 📄 Licencia

MIT. Ver [LICENSE](LICENSE).

---

<!-- Badges -->

[badge-go]: https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go
[badge-license]: https://img.shields.io/badge/Licencia-MIT-green?style=flat
[badge-docker]: https://img.shields.io/badge/Docker-Compose-2496ED?style=flat&logo=docker
