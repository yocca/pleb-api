# venue-search Specification

## Purpose
Lets clients find bars and restaurants near a location and see their happy hour schedules, including whether a happy hour is running at a given moment.

## Requirements

### Requirement: Radius search
`GET /v1/venues` SHALL return venues within `radius_m` meters (default 1500, max 25000) of the required `lat`/`lng`, ordered by distance ascending, each with its `distance_m`.

#### Scenario: Venues inside the radius are returned nearest first
- **WHEN** a client requests `/v1/venues?lat=40.7336&lng=-74.0027&radius_m=500`
- **THEN** the response lists only venues within 500 m of that point, ordered by increasing `distance_m`

#### Scenario: Missing coordinates
- **WHEN** a client requests `/v1/venues` without `lat` or `lng`
- **THEN** the API responds 400 with error code `invalid_param`

#### Scenario: Radius above the maximum
- **WHEN** a client requests `radius_m=50000`
- **THEN** the API responds 400 with error code `invalid_param`

### Requirement: Venues without visible happy hours are excluded from search
Search results SHALL include only venues that have at least one visible happy hour. A happy hour is visible when it is verified or its confidence is at least 0.6.

#### Scenario: Low-confidence data hidden
- **WHEN** a venue's only happy hour has confidence 0.4 and is not verified
- **THEN** that venue does not appear in search results

#### Scenario: Verified data shown regardless of confidence
- **WHEN** a venue's happy hour has confidence 0.3 and is verified
- **THEN** that venue appears in search results with that happy hour

### Requirement: Active-at filter
When `active_at` (RFC 3339) is given, search SHALL return only venues with a visible happy hour running at that instant, evaluated in each venue's own timezone. A window whose end time is earlier than its start time runs past midnight into the next day.

#### Scenario: Happy hour in progress
- **WHEN** a venue in `America/New_York` has a Tuesday 16:00–19:00 happy hour and `active_at` is Tuesday 17:30 New York time
- **THEN** the venue is returned with `active_now: true`

#### Scenario: Window crossing midnight
- **WHEN** a venue has a Friday 22:00–01:00 happy hour and `active_at` is Saturday 00:30 in the venue's timezone
- **THEN** the venue is returned

#### Scenario: Outside every window
- **WHEN** `active_at` falls outside all of a venue's happy hour windows
- **THEN** the venue is not returned

### Requirement: Day filter
When `day` (0 = Sunday … 6 = Saturday) is given, search SHALL return only venues with a visible happy hour on that weekday.

#### Scenario: Weekday match
- **WHEN** a client requests `day=1` and a venue has happy hours only on Saturday
- **THEN** that venue is not returned

### Requirement: Cursor pagination
Search SHALL return at most `limit` venues (default 50, max 200) and a `next_cursor` that, when passed back as `cursor`, continues the same ordering without duplicates; `next_cursor` SHALL be null on the last page.

#### Scenario: Paging through results
- **WHEN** 120 venues match and the client pages with `limit=50`
- **THEN** three pages return 50, 50 and 20 venues with no venue repeated, and the third page has `next_cursor: null`

### Requirement: Venue representation
Each venue SHALL include `id`, `name`, `address`, `location` (`lat`, `lng`), `timezone`, `active_now` (evaluated at `active_at` or the current time), and its visible `happy_hours`, each with `day_of_week`, `start`, `end` (HH:MM), `all_day`, `deals`, `notes`, `confidence`, `verified`, `source_kind`, `source_url` and `observed_at`.

#### Scenario: Provenance exposed
- **WHEN** a happy hour came from a venue website crawl
- **THEN** it is returned with `source_kind: "website"` and the page's `source_url`

### Requirement: Venue detail
`GET /v1/venues/{id}` SHALL return one venue in the same representation plus `website`, `phone`, `updated_at` and `attribution`. It SHALL return the venue even when it has no visible happy hours.

#### Scenario: Known venue
- **WHEN** a client requests an existing venue id
- **THEN** the API responds 200 with that venue

#### Scenario: Unknown venue
- **WHEN** a client requests an id that does not exist, or is not a valid UUID
- **THEN** the API responds 404 with error code `not_found`
