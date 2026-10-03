# Importing venues

`pleb-api import venues` builds the venue list for an area from two open data sources:

- **OpenStreetMap**: bars, pubs, biergartens and restaurants, with name, coordinates, address, website and phone. The data is licensed under ODbL. Every imported venue carries the attribution `© OpenStreetMap contributors`, which the API returns on venue detail and which any screen showing these venues must display. If we ever publish the venue database itself (not just the app), it must be shared under ODbL.
- **NY State Liquor Authority licenses** (NY open data): used to confirm a venue holds an on-premises license and to report licensed places OSM is missing. Licenses never create venues on their own.

Nothing from Google is used.

## 1. Download the data

West Village bounding box (south, west, north, east): `40.7280,-74.0110,40.7400,-73.9990`.

**OpenStreetMap**: save an Overpass export of the area:

```sh
curl -s https://overpass-api.de/api/interpreter --data-urlencode \
  'data=[out:json][timeout:90];nwr["amenity"~"^(bar|pub|biergarten|restaurant)$"](40.7280,-74.0110,40.7400,-73.9990);out center tags;' \
  > osm.json
```

**Liquor licenses**: export the State Liquor Authority's current active licenses dataset from data.ny.gov as CSV (filter to New York County first to keep it small) and save it as `sla.csv`. The importer finds columns by header name, case-insensitively. It needs a serial, a premises name and an address, and uses the DBA, license type, ZIP and latitude/longitude when present. The accepted header names are listed in `licenseColumns` in `internal/importer/sources.go`. If the export's headers differ, the importer stops with an error naming the missing column, so add the new header name there.

Only on-premises licenses (type containing "on premises") are considered. If the CSV has coordinates, licenses are limited to the bounding box. Otherwise pass `--zips`; for the West Village use `10014,10011,10012`.

## 2. Preview, then import

```sh
pleb-api import venues --osm osm.json --sla sla.csv --bbox 40.7280,-74.0110,40.7400,-73.9990 --zips 10014,10011,10012 --dry-run
pleb-api import venues --osm osm.json --sla sla.csv --bbox 40.7280,-74.0110,40.7400,-73.9990 --zips 10014,10011,10012
```

The `--sla` flag is optional. Where the network allows it, `--fetch` downloads the OSM export from Overpass instead of reading `--osm`. Add `--sla-url <csv export url>` to download the licenses too.

To try it against the checked-in sample data (the names are fictional):

```sh
docker compose up -d db migrate
go run ./cmd/pleb-api import venues --osm testdata/osm_west_village.json --sla testdata/sla_west_village.csv \
  --bbox 40.7280,-74.0110,40.7400,-73.9990 --zips 10014 --dry-run
```

The importer only accepts areas inside New York City for now, because every venue is stored with the `America/New_York` timezone.

## 3. Read the report

```
venues: 5 created, 0 updated, 3 skipped
licenses: 3 matched, 1 unmatched
  unmatched license 4444444: MYSTERY BAR LLC (dba MYSTERY LOUNGE), 99 BARROW ST
```

- **created / updated**: venues are keyed by OSM id (`node/123`, `way/456`). Re-running updates them in place and never deletes them or touches their happy hours.
- **skipped**: features that aren't a venue type, have no `name`, or fall outside the box.
- **matched**: a license links to a venue when their street addresses normalize to the same key (so "75 Seventh Avenue South" equals "75 7TH AVE S") and the venue's name has trigram similarity of at least 0.4 to the license's premises name or DBA. The serial is stored on the venue.
- **unmatched**: licensed on-premises places with no matching OSM venue. These are worth checking on a field visit. Either OSM is missing the venue (add it to OpenStreetMap, then re-import) or the names or addresses differ too much to match.
