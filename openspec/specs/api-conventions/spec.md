# api-conventions Specification

## Purpose
Defines behavior every pleb-api endpoint shares, so clients can handle errors, health and the contract the same way everywhere.

## Requirements

### Requirement: Error envelope
Every error response SHALL use the JSON body `{"error": {"code": "<code>", "message": "<text>"}}` with status 400 (`invalid_param`), 404 (`not_found`), 500 (`internal`) or 503 (`unavailable`). A 500 SHALL NOT include internal details such as SQL or stack traces.

#### Scenario: Bad parameter
- **WHEN** a client sends `lat=abc`
- **THEN** the API responds 400 with `{"error": {"code": "invalid_param", "message": ...}}` naming the parameter

#### Scenario: Database failure
- **WHEN** a query fails unexpectedly
- **THEN** the API responds 500 with code `internal` and a generic message

### Requirement: Health check
`GET /healthz` SHALL respond 200 `{"status":"ok"}` when the database is reachable and 503 with the error envelope (code `unavailable`) when it is not.

#### Scenario: Database up
- **WHEN** Postgres is reachable
- **THEN** `/healthz` responds 200 `{"status":"ok"}`

#### Scenario: Database down
- **WHEN** Postgres is unreachable
- **THEN** `/healthz` responds 503 with code `unavailable`

### Requirement: Published contract
The repository SHALL contain an `openapi.yaml` describing every endpoint, parameter, response body and error, and the API's actual responses SHALL conform to it.

#### Scenario: Contract matches behavior
- **WHEN** the contract test suite validates real responses from each endpoint against `openapi.yaml`
- **THEN** every response conforms
