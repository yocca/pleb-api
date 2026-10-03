# Proposal

## Why

Pleb has three empty repos and an agreed plan, but nothing can store or serve a venue yet. The web app and the extraction jobs both need a database schema and a read API before any of their work can be tested, so `pleb-api` comes first, seeded with a real West Village venue list.

## What Changes

- Postgres 16 + PostGIS schema managed by migrations in this repo: `venues`, `submissions`, `extractions`, `happy_hours`. It's the only schema in the system, so `pleb-agent` writes to these tables but never migrates them.
- A venue importer for the launch area. It loads bars, pubs and restaurants from OpenStreetMap, cross-references NY State Liquor Authority on-premises licenses, merges the two into `venues`, and can be re-run safely.
- A read-only HTTP API: `GET /v1/venues` (radius search with optional "happy hour active at" and weekday filters, cursor pagination), `GET /v1/venues/{id}` and `GET /healthz`.
- `openapi.yaml` as the published contract that `pleb-web` generates its types from.
- Local development: a `docker-compose.yml` running PostGIS, migrations and the API, building `../pleb-web` and `../pleb-agent` as sibling services, plus a small hand-written seed so the API returns happy hours before any extraction has run.

## Capabilities

### New Capabilities
- `venue-search`: finding venues near a point and reading one venue, with their visible happy hour schedules and "active now" evaluation in the venue's timezone.
- `venue-import`: building and refreshing the venue list for a launch area from OpenStreetMap and NY liquor license data, with attribution kept.
- `api-conventions`: behavior shared by every endpoint: the error envelope, the health check, and the published OpenAPI contract.

### Modified Capabilities
- None (no existing specs).

## Impact

- New Go module (`github.com/yocca/pleb-api`): HTTP server, migration runner, importer CLI.
- New runtime dependency: Postgres 16 with PostGIS 3.
- External data: OpenStreetMap (ODbL, attribution required) and NY open data liquor license records. Nothing from Google is stored or used.
- Downstream: `pleb-web` consumes `openapi.yaml`, and `pleb-agent` writes to `submissions`, `extractions` and `happy_hours`, so later schema changes to those tables must be coordinated.
