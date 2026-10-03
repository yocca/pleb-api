# Design

## Context

The repo is greenfield: it contains only OpenSpec scaffolding. See proposal.md for scope. Two other services depend on what this change fixes in place: `pleb-agent` writes extraction results to the schema, and `pleb-web` generates types from `openapi.yaml`. This cloud environment cannot reach Overpass or data.ny.gov, so the importer must also work from downloaded files.

## Goals / Non-Goals

**Goals:**
- A schema that holds every v1 source (website, field photo, user upload) and can take owner claims (v2) without a rewrite.
- Search fast enough for one neighborhood to thousands of venues without tuning.
- One command brings up the whole stack locally.

**Non-Goals:**
- Write endpoints (uploads, corrections, claims); `pleb-web` and `pleb-agent` get those later.
- Auth, rate limiting, deployment beyond local compose.
- Venue discovery outside the launch area, and any Google data.

## Decisions

**Go 1.26, chi, pgx v5, sqlc.** (Go 1.26 because current goose and staticcheck require it.) chi is a thin router over `net/http`; sqlc generates typed queries from plain SQL, which suits PostGIS functions that ORMs handle poorly. *Alternatives:* GORM (weak on PostGIS, hides SQL); Echo/Gin (more framework than we need).

**Migrations: goose, SQL files embedded in the binary.** The same `pleb-api migrate up` runs in compose and later in CI or k8s jobs. *Alternative:* golang-migrate, which works just as well; goose's single-binary embedding is slightly simpler.

**One binary, subcommands.** `pleb-api serve | migrate up|down | import venues`. One image serves all three compose roles.

**Schema** (full DDL lands in the migration):
- `venues`: `id uuid`, `osm_id text unique`, `sla_serial text`, `name`, `address`, `location geography(Point,4326)` with a GiST index, `timezone text` (IANA), `website`, `phone`, `attribution text[]`, `crawl_status` (`pending|found|not_listed|manual_only|no_site`), `owner_locked bool` (v2), `google_place_id text null` (unused in v1), timestamps.
- `submissions`: one row per input that can produce a schedule. `kind` (`website|field_photo|user_upload|owner`), `source_url`, `object_key`, `content_sha256`, `submitted_by`, `status`.
- `extractions`: raw model output per submission. `model`, `prompt_version`, `is_happy_hour`, `confidence`, `output jsonb`.
- `happy_hours`: the serving table. `day_of_week smallint`, `start_time time`, `end_time time`, `all_day`, `deals jsonb`, `notes`, `confidence real`, `verified`, `source_kind`, `source_url`, `observed_at`, `extraction_id`.
- Enumerations are `text` with `CHECK` constraints rather than Postgres enums, so adding a value is a one-line migration.

**Visibility lives in a SQL view.** `visible_happy_hours` = `verified OR confidence >= 0.6`. Search and detail both read the view, so the threshold is defined once. *Alternative:* a Go constant applied per query, which risks drift between queries.

**"Active at" is evaluated in SQL.** For each candidate venue: `local := active_at AT TIME ZONE venues.timezone`. A normal window matches when `dow(local) = day_of_week AND start <= time(local) < end`. A window with `end < start` also matches `dow(local) = day_of_week+1 mod 7 AND time(local) < end`. `all_day` matches the whole weekday. This keeps pagination and filtering in one query. *Alternative:* filtering in Go after the query, which breaks cursor pages.

**Search query shape:** `ST_DWithin(location, point, radius)` on the GiST index, `EXISTS` against the visibility view (plus the active and day predicates), ordered by `(distance, id)`. The cursor is a base64 `(distance, id)` keyset, which keeps pages stable without OFFSET. Happy hours for the page's venues are fetched in a second query with `venue_id = ANY($1)`.

**Importer reads files, with optional fetch.** `import venues --osm <file.json> --sla <file.csv> --bbox s,w,n,e [--dry-run]`. A `--fetch` flag downloads from Overpass and data.ny.gov where the network allows. Tests use small checked-in fixtures in the Overpass and SLA CSV formats. These are hand-written with fictional names, because Overpass and data.ny.gov weren't reachable when the fixtures were made. SLA columns are found by header name with aliases, since the export's exact headers couldn't be verified. Address normalization handles street-suffix abbreviations, ordinals and unit stripping; names are matched with trigram similarity ≥ 0.4, computed in Go with pg_trgm's algorithm (a test checks it against `pg_trgm`'s `similarity()`) so dry runs and matching need no database round-trips and the schema needs no `pg_trgm` extension. Timezone is fixed to `America/New_York` for NYC imports; deriving it from coordinates waits until we expand beyond one area.

**Contract test:** `openapi.yaml` is hand-written. Integration tests load it with `kin-openapi` and validate every response they receive, which makes the "contract matches behavior" requirement a test.

**Integration tests run against real PostGIS** via `testcontainers-go` (or `PLEB_TEST_DATABASE_URL`), with a fresh database per test, not mocks, since the riskiest logic (distance, timezone, midnight windows) is in SQL.

**Local dev:** `docker-compose.yml` here with `db` (`postgis/postgis:16-3.4`, healthcheck), `migrate` (runs once), `seed` (loads `dev/seed.sql` into a fresh DB), `api` (port 8080), and `web` and `agent` building from `../pleb-web` and `../pleb-agent` under compose profiles so the API works without them. Seed data marks itself `source_kind = 'field_photo'`, `verified = true`, with obviously fictional venue names, so it can't be mistaken for real data.

## Risks / Trade-offs

- [OSM coverage or website tags are thin in the West Village] → The importer's report shows counts. Field visits fill gaps by adding venues manually via seed-style SQL until an admin path exists.
- [Fuzzy SLA matching links the wrong license] → License serial is informational in v1 (nothing depends on it), and the unmatched report is reviewed by hand.
- [Timezone hard-coded to New York] → Explicit flag, and the importer refuses bboxes outside NYC until timezone lookup is added.
- [Schema churn once pleb-agent is built] → Text enums and additive migrations; pleb-agent changes to these tables go through an OpenSpec change here.
- [testcontainers needs Docker in CI] → GitHub Actions ubuntu runners have it; locally the compose DB can be used via `DATABASE_URL`.

## Migration Plan

Greenfield, nothing to migrate. Rollback is `pleb-api migrate down` to zero locally.
