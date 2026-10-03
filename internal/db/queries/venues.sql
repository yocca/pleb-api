-- name: SearchVenues :many
-- Venues within radius_m of a point that have at least one visible happy hour
-- matching the optional day and active_at filters, keyset-paginated by (distance, id).
WITH origin AS (
    SELECT ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326)::geography AS pt
),
candidates AS (
    SELECT v.id, v.name, v.address, v.timezone, v.location,
           ST_Distance(v.location, o.pt)::float8 AS distance_m
    FROM venues v, origin o
    WHERE ST_DWithin(v.location, o.pt, @radius_m::float8)
)
SELECT c.id, c.name, c.address, c.timezone,
       ST_Y(c.location::geometry)::float8 AS lat,
       ST_X(c.location::geometry)::float8 AS lng,
       c.distance_m,
       EXISTS (
           SELECT 1 FROM visible_happy_hours h
           WHERE h.venue_id = c.id
             AND pleb_hh_active(h.day_of_week, h.start_time, h.end_time, h.all_day,
                                sqlc.arg(evaluated_at)::timestamptz AT TIME ZONE c.timezone)
       )::boolean AS active_now
FROM candidates c
WHERE EXISTS (
        SELECT 1 FROM visible_happy_hours h
        WHERE h.venue_id = c.id
          AND (sqlc.narg(day)::smallint IS NULL OR h.day_of_week = sqlc.narg(day)::smallint)
          AND (sqlc.narg(active_at)::timestamptz IS NULL
               OR pleb_hh_active(h.day_of_week, h.start_time, h.end_time, h.all_day,
                                 sqlc.narg(active_at)::timestamptz AT TIME ZONE c.timezone))
      )
  AND (sqlc.narg(after_distance)::float8 IS NULL
       OR (c.distance_m, c.id) > (sqlc.narg(after_distance)::float8, sqlc.narg(after_id)::uuid))
ORDER BY c.distance_m, c.id
LIMIT @row_limit::int;

-- name: GetVenue :one
SELECT v.id, v.name, v.address, v.timezone,
       ST_Y(v.location::geometry)::float8 AS lat,
       ST_X(v.location::geometry)::float8 AS lng,
       v.website, v.phone, v.attribution, v.updated_at,
       EXISTS (
           SELECT 1 FROM visible_happy_hours h
           WHERE h.venue_id = v.id
             AND pleb_hh_active(h.day_of_week, h.start_time, h.end_time, h.all_day,
                                sqlc.arg(evaluated_at)::timestamptz AT TIME ZONE v.timezone)
       )::boolean AS active_now
FROM venues v
WHERE v.id = @id;

-- name: VisibleHappyHoursForVenues :many
SELECT venue_id, day_of_week, start_time, end_time, all_day, deals, notes,
       confidence, verified, source_kind, source_url, observed_at
FROM visible_happy_hours
WHERE venue_id = ANY(@venue_ids::uuid[])
ORDER BY venue_id, day_of_week, start_time NULLS FIRST;
