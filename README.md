# pleb-api

Go API for Pleb, a happy hour finder. Serves venues and happy hour schedules from Postgres + PostGIS, owns the database migrations, and holds the docker compose setup for local development.

- API contract: [`openapi.yaml`](openapi.yaml)
- Schema and the rules other services rely on: [`docs/schema.md`](docs/schema.md)
- Importing venues for an area: [`docs/import.md`](docs/import.md)

## Local development

Check out the three repos side by side:

```
pleb/
  pleb-api/     (this repo)
  pleb-web/
  pleb-agent/
```

Start Postgres, run migrations, load the fictional seed venues and serve the API on port 8080:

```sh
docker compose up db migrate seed api
curl 'localhost:8080/v1/venues?lat=40.7336&lng=-74.0027&radius_m=1000'
```

Add the web app with `docker compose --profile web up`, and run an agent job with `docker compose --profile agent run agent <command>` (both build from the sibling repos).

To run the API natively against the compose database instead:

```sh
docker compose up -d db migrate seed
DATABASE_URL='postgres://pleb:pleb@localhost:5432/pleb?sslmode=disable' go run ./cmd/pleb-api serve
```

`DATABASE_URL` defaults to that value, so it can be left out locally. `pleb-api migrate up|down|reset|status` manages the schema.

### Tests

```sh
go test ./...
```

Integration tests start a PostGIS container with testcontainers, so Docker must be running. To reuse a server you already have, point `PLEB_TEST_DATABASE_URL` at a user that can create databases, e.g. `PLEB_TEST_DATABASE_URL='postgres://pleb:pleb@localhost:5432/postgres?sslmode=disable'`.

### Regenerating queries

Queries live in `internal/db/queries` and are compiled to Go with [sqlc](https://sqlc.dev). sqlc checks them against a live database because PostGIS functions aren't in its built-in catalog:

```sh
docker compose up -d db migrate
SQLC_DATABASE_URL='postgres://pleb:pleb@localhost:5432/pleb?sslmode=disable' sqlc generate
```

## Specs

This repo uses [OpenSpec](https://github.com/Fission-AI/OpenSpec) for spec-driven changes. Project context and sourcing rules are in `openspec/config.yaml`; current specs are in `openspec/specs/`, proposed changes in `openspec/changes/`. In Claude Code, use `/opsx:propose`, `/opsx:apply` and `/opsx:archive`.
