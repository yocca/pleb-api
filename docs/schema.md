# Database schema

Postgres 16 with PostGIS. The schema lives only in this repo, in `internal/db/migrations` (goose, embedded in the binary). Other services write to some tables but never migrate them; schema changes go through an OpenSpec change here.

Run migrations with `pleb-api migrate up` (or `down`, `reset`, `status`).

## Tables

### `venues`

Our own venue record. v1 venues come from OpenStreetMap (`osm_id`, e.g. `node/123`), optionally linked to a NY State Liquor Authority license (`sla_serial`). Nothing Google-sourced is stored. `google_place_id` exists for a possible later link and is unused in v1.

| Column | Notes |
|---|---|
| `location` | `geography(Point, 4326)`, GiST-indexed for radius search |
| `timezone` | IANA name; happy hour times are wall-clock times in this zone |
| `attribution` | credits the API shows on venue detail, e.g. `© OpenStreetMap contributors` |
| `crawl_status` | `pending`, `found`, `not_listed`, `manual_only` or `no_site` (set by pleb-agent) |
| `owner_locked` | v2: a verified owner's data can't be overwritten by user uploads |

### `submissions` (written by pleb-agent and, later, the upload flow)

Every input that can produce a schedule. `kind` is `website`, `field_photo`, `user_upload` or `owner`. Photos are referenced by `object_key` in private object storage, never stored in the database.

### `extractions` (written by pleb-agent)

Raw model output for one submission: `model`, `prompt_version`, `is_happy_hour`, `confidence` (0–1) and `output` (JSON). This is kept so schedules can be audited and re-derived when prompts change.

### `happy_hours` (written by pleb-agent, read by the API)

The serving table: one row per weekday window.

| Column | Notes |
|---|---|
| `day_of_week` | 0 = Sunday … 6 = Saturday |
| `start_time`, `end_time` | local wall-clock times; `end_time < start_time` means the window runs past midnight into the next day |
| `all_day` | the whole weekday; start and end may be null |
| `deals` | JSON array, e.g. `[{"item": "Draft beer", "price": 5.0}]` |
| `confidence`, `verified` | see visibility below |
| `source_kind`, `source_url`, `observed_at` | provenance, shown to clients |

## Rules other services rely on

**Visibility.** The API only serves rows from the `visible_happy_hours` view: `verified OR confidence >= 0.6`. Lower-confidence rows are stored for review but hidden.

**Source precedence** (enforced by pleb-agent when it replaces a venue's rows): `owner` > `field_photo` > `website` > `user_upload`. A new extraction replaces a venue's `happy_hours` only if its source ranks at least as high as the current one, or the current rows are older than 60 days. When `owner_locked` is true, user uploads become suggestions instead of edits.

**Active windows.** `pleb_hh_active(day, start, end, all_day, local_timestamp)` is the single definition of whether a window covers a local moment, including the midnight-crossing case. Callers convert to the venue's zone first: `pleb_hh_active(..., at AT TIME ZONE venues.timezone)`.
