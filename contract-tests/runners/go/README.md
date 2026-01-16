# Go Contract Runner (stub)

This runner executes the shared DynamORM contract scenarios from `contract-tests/scenarios/` against the Go DynamORM
implementation (this repo) using DynamoDB Local.

## Prereqs

- Docker running (for DynamoDB Local)

## Run

From the repo root:

```bash
docker compose -f contract-tests/docker-compose.yml up -d
cd contract-tests/runners/go
go test ./... -v
```

## Notes

- This folder is a **nested Go module** so it won’t affect `go test ./...` from the parent repo.
- The `go.mod` uses a local `replace` to point at the parent `dynamorm` module; remove it when extracting to a standalone
  `dynamorm-contract-tests` repo.

