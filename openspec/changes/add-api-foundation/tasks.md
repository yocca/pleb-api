# Tasks

## 1. Project scaffolding

- [x] 1.1 Create the Go module `github.com/yocca/pleb-api` with `cmd/pleb-api` (subcommands `serve`, `migrate`, `import`) and `internal/` packages; verify `go build ./...` and `go vet ./...` pass
- [x] 1.2 Add a multi-stage `Dockerfile` producing a static binary image; verify `docker build .` succeeds and `docker run <img> --help` lists the subcommands
- [ ] 1.3 Add a GitHub Actions workflow running `go vet`, `staticcheck` and `go test ./...`; verify it passes on the PR

## 2. Schema and migrations

- [x] 2.1 Write the initial goose migration (extensions `postgis`, `pgcrypto`; tables `venues`, `submissions`, `extractions`, `happy_hours`; CHECK constraints; GiST and lookup indexes; `visible_happy_hours` view) embedded in the binary; verify `migrate up`, `migrate down` and `migrate up` again all succeed against PostGIS in an integration test
- [x] 2.2 Add a testcontainers-go helper that starts PostGIS and applies migrations for integration tests; verify a smoke test inserts and reads a venue
- [x] 2.3 Document the schema (tables, source precedence, visibility rule, which tables pleb-agent writes) in `docs/schema.md`; verify it matches the migration's table and column names

## 3. Local development

- [x] 3.1 Add `docker-compose.yml` (`db`, `migrate`, `seed`, `api`, plus `web` and `agent` under profiles) and `dev/seed.sql` with a handful of fictional West Village venues and happy hours, including one crossing midnight; verify `docker compose up db migrate seed api` serves seeded venues at `localhost:8080/v1/venues`
- [x] 3.2 Write the README's local dev section (sibling checkout, compose commands, `DATABASE_URL`, running the API natively); verify each documented command runs as written

## 4. API conventions

- [x] 4.1 Implement the error envelope and the panic and 500 handling middleware; verify unit tests cover 400/404/500/503 shapes and that 500 bodies contain no internal error text
- [x] 4.2 Implement `GET /healthz` with a DB ping; verify integration tests get 200 with the DB up and 503 `unavailable` with it stopped
- [x] 4.3 Write `openapi.yaml` for `/healthz`, `/v1/venues` and `/v1/venues/{id}`, and add a test helper that validates responses against it with kin-openapi; verify the health test's responses pass validation

## 5. Venue search and detail

- [x] 5.1 Write sqlc queries for radius search (keyset on distance and id, visibility, `day` and `active_at` predicates including midnight crossing) and for happy hours by venue ids; verify `sqlc generate` succeeds and the generated code compiles
- [x] 5.2 Implement `GET /v1/venues` with parameter validation and cursor encoding; verify integration tests cover every venue-search scenario (radius ordering, missing or oversized params, hidden low-confidence rows, verified override, active and midnight and outside windows, day filter, 120-venue paging) and that all responses validate against `openapi.yaml`
- [x] 5.3 Implement `GET /v1/venues/{id}` including `website`, `phone`, `updated_at` and `attribution`; verify integration tests for known, unknown and malformed ids pass with contract validation

## 6. Venue import

- [x] 6.1 Add West Village fixtures under `testdata/` (an OSM Overpass JSON sample and a matching SLA CSV sample with at least one unmatched license and one unnamed feature); verify the files parse
- [x] 6.2 Implement `import venues --osm --sla --bbox [--dry-run] [--fetch]` with OSM parsing, address normalization, trigram name matching, upsert by `osm_id` and OSM attribution; verify integration tests cover every venue-import scenario, including the second run not duplicating venues or touching happy hours, and dry run leaving the DB unchanged
- [x] 6.3 Document the import workflow (downloading the Overpass and SLA files, running the import, reading the unmatched report, OSM attribution duty) in `docs/import.md`; verify the documented commands run against the fixtures

## 7. Integration check

- [x] 7.1 From a clean checkout, run `docker compose up`, import the fixtures, and query `/v1/venues` near the West Village; verify imported venues appear on detail and that search shows seeded venues with visible happy hours
