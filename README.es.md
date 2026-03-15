# Heimdall

**Plataforma de exposure intelligence para el análisis de infraestructura de Internet.**

*[English version](README.md)* Este repositorio aporta la base: versionado de datasets RIR, pipeline de importación de scopes, resolución IP→país e inventario por país.

---

## ¿Qué es Heimdall?

Heimdall busca proporcionar una vista estructurada y actualizada del espacio IP global como base para descubrimiento de activos, escaneo de Internet y flujos de exposure intelligence. La fase actual se centra en **datos de scope autoritativos**: descarga y versionado de estadísticas delegadas de los RIR (Regional Internet Registry), importación de bloques IP y ASN por país, y APIs para resolver IPs a país y listar bloques por país.

---

## Qué hace hoy

- **dataset-service** — Descarga y versiona los ficheros de estadísticas delegadas RIR (`delegated-*-latest`). Valida cabeceras, almacena artefactos y los expone por API. Idempotente por registry/serial.
- **scope-service** — Importa bloques desde dataset-service (por `dataset_id`), los persiste y ofrece:
  - **Resolución IP → país** usando un snapshot lógico construido con el último dataset importado de cada RIR (la vista “actual” combina el RIPE, ARIN, APNIC, etc. más recientes, sin mezclar datos obsoletos).
  - **Inventario por país**: bloques y resumen (conteos IPv4/IPv6) por país, con filtrado opcional por `dataset_id` o familia de direcciones.
  - **Sync**: sincronización en un paso que obtiene el último dataset validado por registry desde dataset-service e importa los que falten.
- **Tests** — Tests unitarios e de integración para parser, pipeline de importación, almacenamiento y handlers (pasan con Go y Postgres local opcional).

---

## Arquitectura (actual)

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

- **dataset-service**: Se conecta a FTP/HTTP de los RIR, parsea el formato RIR, guarda artefactos y metadatos en Postgres (`heimdall_datasets`).
- **scope-service**: Obtiene artefactos de dataset-service, parsea y filtra bloques (país, IPv4/IPv6), los guarda en Postgres (`heimdall_scope_service`). Resuelve IPs y sirve el inventario por país usando un **snapshot lógico**: el último dataset importado de cada registry (RIPE, ARIN, APNIC, etc.) se combina en una vista coherente cuando no se indica `dataset_id`.

---

## Servicios

| Servicio          | Función                                                                 | Puerto por defecto |
|-------------------|-------------------------------------------------------------------------|---------------------|
| **dataset-service** | Descarga, valida, versiona y sirve datasets RIR                        | 8080                |
| **scope-service**   | Importa bloques, resuelve IP→país, inventario por país, sync           | 8081                |
| **PostgreSQL**      | Dos bases: `heimdall_datasets`, `heimdall_scope_service`              | 5432                |

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

1. **Postgres** (puerto 5432), creando `heimdall_datasets` y `heimdall_scope_service` en el primer arranque.
2. **dataset-service** (puerto 8080), cuando Postgres está listo.
3. **scope-service** (puerto 8081), cuando dataset-service está listo.

Volúmenes: `postgres_data`, `dataset_artifacts` (ficheros RIR descargados). No hace falta `.env`; todo está definido en `docker-compose.yml`.

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
   ```

---

## Endpoints de ejemplo

### dataset-service (puerto 8080)

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/v1/datasets/fetch?registry=ripencc\|arin\|apnic\|lacnic\|afrinic\|all` | Descarga y registra una nueva versión (idempotente por serial). |
| GET | `/v1/datasets` | Lista versiones de datasets (más recientes primero). |
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
| GET | `/v1/scopes/country/{cc}/datasets` | Datasets importados que tienen bloques para ese país. |
| GET | `/health`, `/ready`, `/version` | Salud y versión. |

Especificaciones OpenAPI: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`.

---

## Desarrollo local (sin Docker)

1. **Crear las bases de datos:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   ```

2. **Entorno:** Copiar `configs/.env.example` a `.env` y configurar DSNs, puertos y `DATASET_SERVICE_URL` (ver comentarios en el ejemplo).

3. **Ejecutar (dos terminales):**

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

   Los tests de integración de storage e import asumen que Postgres está disponible (mismo DSN que en `.env` o valores por defecto en el código). Si la DB no está disponible, se hace skip.

---

## Estado del proyecto

- **Fase:** Desarrollo. Enfoque actual: **inventario RIR y resolución de scopes** (datasets, import, by-IP, bloques/resumen por país, sync multi-RIR).
- **Aún no:** Un motor de escaneo completo ni un pipeline de exposure; este repo es la base de datos y APIs para ese tipo de sistema.

---

## Licencia

MIT. Ver [LICENSE](LICENSE).
