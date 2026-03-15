# Heimdall

**Plataforma de exposure intelligence para el análisis de infraestructura de Internet.**

*[English version](README.md)* Este repositorio aporta la base: versionado de datasets RIR, pipeline de importación de scopes, resolución IP→país e inventario por país.

---

## ¿Qué es Heimdall?

Heimdall busca proporcionar una vista estructurada y actualizada del espacio IP global como base para descubrimiento de activos, escaneo de Internet y flujos de exposure intelligence. La fase actual se centra en **datos de scope autoritativos**: descarga y versionado de estadísticas delegadas de los RIR (Regional Internet Registry), importación de bloques IP y ASN por país, y APIs para resolver IPs a país y listar bloques por país.

---

## Qué hace hoy

- **dataset-service** — Descarga y versiona estadísticas delegadas RIR y fuentes CAIDA (pfx2as IPv4/IPv6, as-org). Valida cabeceras, almacena artefactos y los expone por API. Idempotente por registry/serial o source/source_version.
- **scope-service** — **Inventario de recursos asignados (RIR).** Importa bloques desde dataset-service (por `dataset_id`), los persiste y ofrece resolución IP→país, inventario por país (bloques, resumen, rangos ASN). Sync obtiene el último validado por registry e importa lo que falte. El inventario ASN refleja asignaciones delegadas por los RIR, no el estado BGP actual.
- **routing-service** — **Estado de routing observado (BGP).** Importa CAIDA pfx2as y AS Organizations desde dataset-service. Ofrece IP→ASN (longest-prefix match), metadata ASN (org/nombre) y ASN→prefijos. Datos derivados de BGP, no de asignación.
- **Tests** — Tests unitarios e de integración para parser, pipeline de importación, almacenamiento y handlers (pasan con Go y Postgres local opcional).

---

## Arquitectura (actual)

```
                    +------------------+
                    |   PostgreSQL     |
                    | (3 DBs: datasets |
                    |  scope, routing) |
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

- **dataset-service**: Se conecta a FTP/HTTP de los RIR, parsea el formato RIR, guarda artefactos y metadatos en Postgres (`heimdall_datasets`).
- **scope-service**: Obtiene artefactos de dataset-service, parsea y filtra bloques (país, IPv4/IPv6), los guarda en Postgres (`heimdall_scope_service`). Resuelve IPs y sirve el inventario por país usando un **snapshot lógico**: el último dataset importado de cada registry (RIPE, ARIN, APNIC, etc.) se combina en una vista coherente cuando no se indica `dataset_id`.

---

## Servicios

| Servicio            | Función                                                                 | Puerto por defecto |
|---------------------|-------------------------------------------------------------------------|---------------------|
| **dataset-service** | Descarga, valida, versiona datasets RIR y CAIDA; sirve artefactos       | 8080                |
| **scope-service**   | **Inventario RIR.** Importa bloques, IP→país, inventario país/ASN, sync | 8081                |
| **routing-service** | **Estado de routing.** Importa pfx2as + as-org, IP→ASN (LPM), metadata ASN, ASN→prefijos | 8082                |
| **PostgreSQL**      | Bases: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service` | 5432          |

---

## Requisitos

- **Go 1.22+** (para compilar y tests en local)
- **PostgreSQL 15+** (para ejecución local o vía Docker)
- **Docker y Docker Compose** (recomendado para levantar el stack)

---

## Arranque rápido (Docker)

Desde la raíz del proyecto:

```bash
cd deployments/docker
docker compose up --build -d
```

Se levantan:

1. **Postgres** (puerto 5432), creando `heimdall_datasets`, `heimdall_scope_service` y `heimdall_routing_service` en el primer arranque.
2. **dataset-service** (puerto 8080), cuando Postgres está listo.
3. **scope-service** (puerto 8081) y **routing-service** (puerto 8082), cuando dataset-service está listo.

Volúmenes: `postgres_data`, `dataset_artifacts`. No hace falta `.env`; todo está definido en `docker-compose.yml`.

---

## Flujo de uso básico

1. **Fetch** de uno o más datasets RIR (p. ej. RIPE, ARIN, APNIC):

   ```bash
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=ripencc"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=arin"
   curl -X POST "http://localhost:8080/v1/datasets/fetch?registry=all"
   ```

2. **Sync** de scope-service con el último dataset validado por registry (importa lo que falte):

   ```bash
   curl -X POST "http://localhost:8081/v1/imports/sync"
   ```

   O importar un dataset concreto:

   ```bash
   curl -X POST "http://localhost:8081/v1/import?dataset_id=<uuid>"
   ```

3. **Resolución** de una IP a país:

   ```bash
   curl "http://localhost:8081/v1/scopes/by-ip/8.8.8.8"
   ```

4. **Inventario por país** (bloques y resumen):

   ```bash
   curl "http://localhost:8081/v1/scopes/country/ES/summary"
   curl "http://localhost:8081/v1/scopes/country/ES/blocks?limit=10"
   curl "http://localhost:8081/v1/scopes/country/ES/asns?limit=10"
   curl "http://localhost:8081/v1/scopes/country/ES/asn-summary"
   ```

5. **Routing (estado observado)** — Fetch CAIDA, sync de routing-service, IP→ASN y metadata/prefijos:

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

**heimdallctl** es la CLI oficial para operar Heimdall. Se comunica con dataset-, scope- y routing-service por HTTP (sin lógica de negocio ni base de datos en la CLI). Salida legible por defecto; usa `-o json` para scripting.

**Compilar:** (el binario no se sube al repo; usa `bin/` o la ruta que prefieras)

```bash
mkdir -p bin && go build -o bin/heimdallctl ./cmd/heimdallctl
```

**Configuración:** Las variables de entorno tienen prioridad sobre los archivos de configuración. URLs base por defecto: `http://localhost:8080` (dataset), `http://localhost:8081` (scope), `http://localhost:8082` (routing). Archivo opcional: `~/.config/heimdall/config.yaml` o `.heimdall.yaml` en el directorio actual. Variables: `HEIMDALL_DATASET_URL`, `HEIMDALL_SCOPE_URL`, `HEIMDALL_ROUTING_URL`, `HEIMDALL_TIMEOUT` (segundos).

**Ejemplos:**

```bash
# Estado de los tres servicios
heimdallctl status

# Dataset: fetch (RIR o CAIDA), list, get
heimdallctl dataset fetch --registry=all
heimdallctl dataset fetch --source=caida_pfx2as_ipv4
heimdallctl dataset list
heimdallctl dataset get <uuid>

# Scope: sync, resolver IP, summary/blocks/asns por país
heimdallctl scope sync
heimdallctl scope by-ip 8.8.8.8
heimdallctl scope country summary ES
heimdallctl scope country blocks ES --limit=10
heimdallctl scope country asns ES
heimdallctl scope country asn-summary ES
heimdallctl scope country datasets ES

# Routing: sync, IP→ASN, metadata ASN, prefijos ASN
heimdallctl routing sync
heimdallctl routing by-ip 8.8.8.8
heimdallctl routing asn 15169
heimdallctl routing asn prefixes 15169 --limit=10

# Salida JSON para scripting
heimdallctl status -o json
heimdallctl scope by-ip 8.8.8.8 -o json
```

Consulta las especificaciones OpenAPI de cada servicio para la semántica completa.

---

## Endpoints de ejemplo

### dataset-service (puerto 8080)

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/v1/datasets/fetch?registry=...` o `?source=caida_pfx2as_ipv4\|caida_pfx2as_ipv6\|caida_as_org` | Fetch RIR o CAIDA (idempotente). |
| GET | `/v1/datasets?source=&source_type=` | Lista versiones (filtros opcionales: source, source_type rir\|caida). |
| GET | `/v1/datasets/{id}` | Metadatos de una versión. |
| GET | `/v1/datasets/{id}/artifact` | Stream del contenido del artefacto. |
| GET | `/health`, `/ready`, `/version` | Salud y versión. |

### scope-service (puerto 8081)

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/v1/import?dataset_id=<uuid>` | Importa un dataset en scope (idempotente). |
| POST | `/v1/imports/sync` | Sync: obtiene el último por registry desde dataset-service e importa los que falten. |
| GET | `/v1/scopes/by-ip/{ip}` | Resuelve IP a país (opcional `?dataset_id=`). |
| GET | `/v1/scopes/country/{cc}/blocks` | Bloques por país (opcional `dataset_id`, `address_family`, `limit`, `offset`). |
| GET | `/v1/scopes/country/{cc}/summary` | Conteos IPv4/IPv6/total por país. |
| GET | `/v1/scopes/country/{cc}/asns` | Rangos ASN por país (delegados RIR; no BGP/IP→ASN). |
| GET | `/v1/scopes/country/{cc}/asn-summary` | asn_range_count y asn_total_count por país. |
| GET | `/v1/scopes/country/{cc}/datasets` | Datasets importados que tienen bloques para ese país. |
| GET | `/health`, `/ready`, `/version` | Salud y versión. |

### routing-service (puerto 8082)

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/v1/imports/sync` | Sync: obtiene último CAIDA (pfx2as IPv4/IPv6, as-org) desde dataset-service e importa. |
| GET | `/v1/asn/by-ip/{ip}` | IP→ASN (longest-prefix match); opcional `?dataset_id=`. |
| GET | `/v1/asn/{asn}` | Metadata ASN (as_name, org_name); 404 si no hay. |
| GET | `/v1/asn/prefixes/{asn}` | Prefijos con primary_asn = ASN; `limit`, `offset`, `dataset_id`. |
| GET | `/health`, `/ready`, `/version` | Salud y versión. |

Especificaciones OpenAPI: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`, `api/openapi/routing-service.yaml`.

---

## Desarrollo local (sin Docker)

1. **Crear las bases de datos:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   CREATE DATABASE heimdall_routing_service;
   ```

2. **Entorno:** Copiar `configs/.env.example` a `.env` y configurar DSNs, puertos y `DATASET_SERVICE_URL` (ver comentarios en el ejemplo).

3. **Ejecutar (tres terminales):**

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

   Los tests de integración de storage e import asumen que Postgres está disponible (mismo DSN que en `.env` o valores por defecto en el código). Si la DB no está disponible, se hace skip.

---

## Estado del proyecto

- **Fase:** Desarrollo. **scope-service:** inventario RIR asignado. **routing-service:** estado de routing observado (BGP). Datasets RIR + CAIDA, resolución IP→país e IP→ASN, inventario país/ASN, metadata ASN y prefijos.
- **Aún no:** Motor de escaneo completo ni pipeline de exposure; este repo es la base de datos y APIs para ese tipo de sistema.

---

## Licencia

MIT. Ver [LICENSE](LICENSE).
