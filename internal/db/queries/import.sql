-- name: VenueKeysByOSMIDs :many
SELECT id, osm_id FROM venues WHERE osm_id = ANY(@osm_ids::text[]);

-- name: UpsertOSMVenue :one
-- Inserts or refreshes a venue from OpenStreetMap. A license serial is only overwritten
-- when this run matched one, and happy hours are never touched.
INSERT INTO venues (osm_id, sla_serial, name, address, location, timezone, website, phone, attribution)
VALUES (@osm_id, sqlc.narg(sla_serial),
        @name, sqlc.narg(address),
        ST_SetSRID(ST_MakePoint(@lng::float8, @lat::float8), 4326)::geography,
        @timezone, sqlc.narg(website), sqlc.narg(phone), @attribution::text[])
ON CONFLICT (osm_id) DO UPDATE SET
    sla_serial  = COALESCE(EXCLUDED.sla_serial, venues.sla_serial),
    name        = EXCLUDED.name,
    address     = EXCLUDED.address,
    location    = EXCLUDED.location,
    timezone    = EXCLUDED.timezone,
    website     = EXCLUDED.website,
    phone       = EXCLUDED.phone,
    attribution = EXCLUDED.attribution,
    updated_at  = now()
RETURNING id, (xmax = 0)::boolean AS inserted;
