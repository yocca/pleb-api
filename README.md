# pleb-api

Go API for Pleb, a happy hour finder. Serves venues and happy hour schedules from Postgres + PostGIS, owns the database migrations, and holds the docker compose setup for local development.

## Local development

Check out `pleb-web` and `pleb-agent` next to this repo, then run `docker compose up` here (the compose file lands with the first change).

## Specs

This repo uses [OpenSpec](https://github.com/Fission-AI/OpenSpec) for spec-driven changes. Project context and sourcing rules are in `openspec/config.yaml`; current specs are in `openspec/specs/`, proposed changes in `openspec/changes/`. In Claude Code, use `/opsx:propose`, `/opsx:apply` and `/opsx:archive`.
