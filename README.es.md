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
- **target-service** — Definiciones de targets (include/exclude por país, ASN, prefijo, world), materialización a snapshots CIDR y diff entre snapshots. Consume scope y routing. Snapshots inmutables. **Exclusión por prefijo (v1):** solo elimina prefijos materializados contenidos en el CIDR de exclusión; *no* hace sustracción CIDR ni subdivisión.
- **Tests** — Tests unitarios e de integración para parser, pipeline de importación, almacenamiento y handlers (pasan con Go y Postgres local opcional).

---

## Arquitectura (actual)

```
                         +------------------+
                         |   PostgreSQL     |
                         | (4 DBs: datasets,|
                         |  scope, routing, |
                         |  target)         |
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
     |                                                    |  diff)    |          +------------+
     | FTP/HTTP (RIR)                                     +-----------+
RIPE NCC, ARIN, APNIC, LACNIC, AFRINIC

heimdallctl (CLI)  ----HTTP---->  dataset  scope  routing  target
```

- **dataset-service**: Se conecta a FTP/HTTP de los RIR, parsea el formato RIR, guarda artefactos y metadatos en Postgres (`heimdall_datasets`). También descarga datasets CAIDA para routing.
- **scope-service**: Obtiene artefactos de dataset-service, parsea y filtra bloques (país, IPv4/IPv6), los guarda en Postgres (`heimdall_scope_service`). Resuelve IPs y sirve el inventario por país usando un **snapshot lógico**: el último dataset importado de cada registry (RIPE, ARIN, APNIC, etc.) se combina en una vista coherente cuando no se indica `dataset_id`.
- **routing-service**: Obtiene CAIDA (pfx2as, as-org) de dataset-service, guarda en Postgres (`heimdall_routing_service`). Expone IP→ASN (LPM), metadata ASN, ASN→prefijos.
- **target-service**: Guarda definiciones de targets (reglas include/exclude por país, ASN, prefijo, world) en Postgres (`heimdall_target_service`). Materializa llamando a scope y routing, persiste snapshots CIDR inmutables. Diff entre snapshots. **World** = unión del inventario RIR por país (vista operativa). **Exclusión por prefijo:** v1 solo quita prefijos materializados contenidos en el CIDR de exclusión; no hay sustracción CIDR.
- **heimdallctl**: CLI oficial; consulta dataset-, scope-, routing- y target-service por HTTP. Sin base de datos ni lógica de negocio; compilar con `go build -o bin/heimdallctl ./cmd/heimdallctl`.

---

## Servicios

| Servicio            | Función                                                                 | Puerto por defecto |
|---------------------|-------------------------------------------------------------------------|---------------------|
| **dataset-service** | Descarga, valida, versiona datasets RIR y CAIDA; sirve artefactos       | 8080                |
| **scope-service**   | **Inventario RIR.** Importa bloques, IP→país, inventario país/ASN, sync | 8081                |
| **routing-service** | **Estado de routing.** Importa pfx2as + as-org, IP→ASN (LPM), metadata ASN, ASN→prefijos | 8082                |
| **target-service**  | Definiciones de targets, reglas, materialización a CIDR, snapshots y diff       | 8083          |
| **PostgreSQL**      | Bases: `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service`, `heimdall_target_service` | 5432   |

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

1. **Postgres** (puerto 5432), creando `heimdall_datasets`, `heimdall_scope_service`, `heimdall_routing_service` y `heimdall_target_service` en el primer arranque.
2. **dataset-service** (puerto 8080), cuando Postgres está listo.
3. **scope-service** (puerto 8081) y **routing-service** (puerto 8082), cuando dataset-service está listo.
4. **target-service** (puerto 8083), cuando scope y routing están listos.

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

6. **Target service** — Crear un target (p. ej. un país), materializarlo a snapshot CIDR, listar snapshots y prefijos:

   ```bash
   curl -X POST "http://localhost:8083/v1/targets" -H "Content-Type: application/json" -d '{"name":"Spain","rules":[{"kind":"include","selector_type":"country","selector_value":"ES"}]}'
   # Usar el id del target devuelto:
   curl -X POST "http://localhost:8083/v1/targets/<target_id>/materialize"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations?limit=10"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations/<mid>/prefixes?limit=10"
   curl "http://localhost:8083/v1/targets/<target_id>/materializations/diff?from=<mid1>&to=<mid2>"
   ```

   **Nota:** La exclusión por prefijo (v1) solo elimina prefijos materializados contenidos en el CIDR de exclusión; el servicio *no* hace sustracción ni subdivisión CIDR.

---

## Heimdallctl (CLI)

**heimdallctl** es la CLI oficial para operar Heimdall. Se comunica con dataset-, scope- y routing-service por HTTP (sin lógica de negocio ni base de datos en la CLI). Salida legible por defecto; usa `-o json` para scripting.

**Compilar:** (el binario no se sube al repo; usa `bin/` o la ruta que prefieras)

```bash
mkdir -p bin && go build -o bin/heimdallctl ./cmd/heimdallctl
```

**Configuración:** Las variables de entorno tienen prioridad sobre los archivos de configuración. URLs base por defecto: `http://localhost:8080` (dataset), `http://localhost:8081` (scope), `http://localhost:8082` (routing), `http://localhost:8083` (target). Archivo opcional: `~/.config/heimdall/config.yaml` o `.heimdall.yaml` en el directorio actual. Variables: `HEIMDALL_DATASET_URL`, `HEIMDALL_SCOPE_URL`, `HEIMDALL_ROUTING_URL`, `HEIMDALL_TARGET_URL`, `HEIMDALL_TIMEOUT` (segundos).

**Ejemplos:**

```bash
# Estado de los cuatro servicios
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

# Target: create, list, get, update, materialize, diff
heimdallctl target list
heimdallctl target create --name Spain --rule include,country,ES
heimdallctl target get <target-id>
heimdallctl target update <target-id> --rule include,country,ES --rule include,country,PT
heimdallctl target materialize <target-id>
heimdallctl target materializations <target-id>
heimdallctl target prefixes <target-id> <materialization-id>
heimdallctl target diff <target-id> --from <mid1> --to <mid2>

# Salida JSON para scripting
heimdallctl status -o json
heimdallctl scope by-ip 8.8.8.8 -o json
```

Consulta las especificaciones OpenAPI de cada servicio (`api/openapi/*.yaml`) para la semántica completa.

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

### target-service (puerto 8083)

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/v1/targets` | Crear target (name, description, rules). |
| GET | `/v1/targets` | Listar targets (por defecto solo activos; `?include_inactive=true`, `limit`, `offset`). |
| GET | `/v1/targets/{id}` | Obtener target con reglas. |
| PUT | `/v1/targets/{id}` | Reemplazo completo de la definición. |
| DELETE | `/v1/targets/{id}` | Soft delete (idempotente). |
| POST | `/v1/targets/{id}/materialize` | Ejecutar materialización (v1 síncrono). |
| GET | `/v1/targets/{id}/materializations` | Listar snapshots (paginado). |
| GET | `/v1/targets/{id}/materializations/{mid}` | Metadatos del snapshot (sin prefijos). |
| GET | `/v1/targets/{id}/materializations/{mid}/prefixes` | Prefijos (paginado). |
| GET | `/v1/targets/{id}/materializations/diff?from=&to=` | Diff entre dos snapshots (mismo target). |
| GET | `/health`, `/ready`, `/version` | Salud y versión. |

Especificaciones OpenAPI: `api/openapi/dataset-service.yaml`, `api/openapi/scope-service.yaml`, `api/openapi/routing-service.yaml`, `api/openapi/target-service.yaml`.

---

## Desarrollo local (sin Docker)

1. **Crear las bases de datos:**

   ```sql
   CREATE DATABASE heimdall_datasets;
   CREATE DATABASE heimdall_scope_service;
   CREATE DATABASE heimdall_routing_service;
   CREATE DATABASE heimdall_target_service;
   ```

2. **Entorno:** Copiar `configs/.env.example` a `.env` y configurar DSNs, puertos y `DATASET_SERVICE_URL` (ver comentarios en el ejemplo).

3. **Ejecutar (cuatro terminales):**

   ```bash
   # Terminal 1 – dataset-service (PORT=8080)
   go run ./cmd/dataset-service

   # Terminal 2 – scope-service (PORT=8081)
   go run ./cmd/scope-service

   # Terminal 3 – routing-service (PORT=8082)
   go run ./cmd/routing-service

   # Terminal 4 – target-service (PORT=8083; necesita scope + routing)
   go run ./cmd/target-service
   ```

4. **Tests:**

   ```bash
   go test ./...
   ```

   Los tests de integración usan **bases de datos de test separadas** (`heimdall_*_test`) por defecto para no tocar los datos de desarrollo. Se crean con el mismo `init-db.sql` al usar Docker. Si Postgres o las DBs de test no están disponibles, los tests hacen skip. Puedes sobreescribir con `DATASET_DB_DSN`, `SCOPE_DB_DSN`, `TARGET_DB_DSN`, etc. para apuntar a tu instancia de test.

---

## Estado del proyecto

- **Fase:** Desarrollo. **scope-service:** inventario RIR asignado. **routing-service:** estado de routing observado (BGP). **target-service:** definiciones de targets, materialización a snapshots CIDR, diff. Datasets RIR + CAIDA, resolución IP→país e IP→ASN, inventario país/ASN, metadata ASN y prefijos.
- **Aún no:** Motor de escaneo completo ni pipeline de exposure; este repo es la base de datos y APIs para ese tipo de sistema.

---

## Licencia

MIT. Ver [LICENSE](LICENSE).
