# Spec Delta

## Purpose

Builds and refreshes Pleb's own venue list for a launch area from open data sources, so venues can be stored without relying on Google content.

## ADDED Requirements

### Requirement: Import venues for an area
The importer SHALL load bars, pubs, biergartens and restaurants inside a given bounding box from an OpenStreetMap export and store each as a venue with its name, address, coordinates, timezone, and website and phone when present.

#### Scenario: West Village import
- **WHEN** the importer runs on an OSM export covering the West Village bounding box
- **THEN** every `amenity` of bar, pub, biergarten or restaurant inside the box exists as a venue with its `osm_id`

#### Scenario: Features without a name are skipped
- **WHEN** an OSM feature has no `name` tag
- **THEN** no venue is created for it and the importer reports it as skipped

### Requirement: Re-running is safe
Importing the same area again SHALL update existing venues in place (matched by `osm_id`) and SHALL NOT create duplicates or delete happy hours attached to those venues.

#### Scenario: Second run
- **WHEN** the importer runs twice on the same export
- **THEN** the venue count is unchanged and existing `happy_hours` rows are untouched

### Requirement: Liquor license cross-reference
The importer SHALL match NY State Liquor Authority on-premises license records to venues by normalized address and fuzzy name, record the license serial on matched venues, and report licenses in the area with no matching venue.

#### Scenario: Licensed bar found in both sources
- **WHEN** an OSM bar and an SLA on-premises license share a normalized street address and similar names
- **THEN** the venue stores that license's serial

#### Scenario: License with no OSM match
- **WHEN** an SLA on-premises license in the area matches no venue
- **THEN** the importer lists it in its unmatched report and does not create a venue from it

### Requirement: Dry run
The importer SHALL support a dry run that reports what would be created, updated, skipped and matched without writing to the database.

#### Scenario: Preview
- **WHEN** the importer runs with `--dry-run`
- **THEN** it prints the counts and the unmatched-license report, and the database is unchanged

### Requirement: Source attribution
Venues created from OpenStreetMap SHALL carry attribution `© OpenStreetMap contributors`, which the API exposes on venue detail.

#### Scenario: Attribution available
- **WHEN** a client reads an imported venue's detail
- **THEN** its `attribution` includes `© OpenStreetMap contributors`
