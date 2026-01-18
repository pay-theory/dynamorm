# Repository Guidelines

## Project Structure & Module Organization
- `dynamorm.go` and other root `*.go`: main `dynamorm` package.
- `pkg/`: public packages (`core/`, `model/`, `query/`, `session/`, `types/`, `marshal/`, `transaction/`, `errors/`, `mocks/`).
- `internal/expr/`: internal-only expression helpers.
- `tests/`: shared test utilities + suites (`tests/integration/`, `tests/benchmarks/`, `tests/stress/`, `tests/models/`).
- `examples/`: runnable examples (including `examples/lambda/`).
- `docs/` and `scripts/`: documentation and helper scripts.

## Build, Test, and Development Commands
Go/tooling:
- Install the Go toolchain declared in `go.mod` (includes a `toolchain` pin).
- If you have Ubuntu snap `go` installed, ensure it doesn't override the pinned toolchain (otherwise you may see `compile: version "goX.Y.Z" does not match go tool version "goX.Y.W"` during coverage/covdata); fix with `export GOTOOLCHAIN="$(awk '/^toolchain /{print $2}' go.mod | head -n1)"` (the `Makefile` already exports this).
- `make install-tools` — install `golangci-lint` and `mockgen`

Common workflows:
- `make fmt` — format (`go fmt` + `gofmt -s -w .`)
- `make lint` — lint (`golangci-lint run ./...`)
- `make test-unit` — fast unit tests (race + coverage; no DynamoDB Local)
- `make unit-cover` — offline coverage baseline (`go test -short ...`)
- `make integration` / `make test` — integration or full suite (starts DynamoDB Local)
- `make benchmark` / `make stress` — performance and stress suites
- `make lambda-build` — build `examples/lambda` → `build/lambda/function.zip`

Single test example: `go test -v -run TestName ./pkg/query`

## Coding Style & Naming Conventions
- Run `make fmt` before pushing; keep changes gofmt-clean.
- Use standard Go naming (exported `PascalCase`, packages `lowercase`).
- Model structs must use canonical tags: `dynamorm:"pk"`/`dynamorm:"sk"` + matching `json:"..."` (see `docs/development-guidelines.md`).

## Testing Guidelines
- Tests use `testing` + `stretchr/testify`; prefer table-driven tests.
- Unit tests should avoid Docker; use interfaces in `pkg/core/` and mocks in `pkg/mocks/`.
- Integration tests rely on DynamoDB Local and `DYNAMODB_ENDPOINT` (see `tests/README.md` and `./tests/setup_test_env.sh`).

## Commit & Pull Request Guidelines
- Branch naming commonly uses `feature/...`, `fix/...`, `chore/...`.
- Prefer Conventional Commit-style subjects (`feat:`, `fix:`, `docs:`, `test:`) and keep the first line ≤72 chars.
- PRs: describe intent and scope, link issues, list commands run, add/adjust tests, and update `CHANGELOG.md` + relevant docs when public APIs change (see `CONTRIBUTING.md`).

### Release promotion (premain → main)
This repo uses two release-please flows:
- `premain`: prerelease (`rc`) via `.release-please-manifest.premain.json`
- `main`: stable via `.release-please-manifest.json`

Because both flows update overlapping version/changelog files, direct `premain` → `main` PRs frequently conflict.

**Preferred promotion workflow (conflict-resistant)**
- Create a temporary branch from `main` (e.g., `promote/premain-to-main-*`) and merge `premain` into it locally.
- Resolve conflicts by keeping `main`’s values for release-managed files (so RC versions don’t leak into stable):
  - `CHANGELOG.md`
  - `ts/package.json`
  - `ts/package-lock.json`
  - `py/src/dynamorm_py/version.json`
  - (usually) `.release-please-manifest.json` and `.release-please-manifest.premain.json`
- Push the promotion branch and open a PR to `main`. Avoid pushing conflict-resolution commits directly to `premain`.
- After merge, `Release (main)` runs and release-please creates/updates the stable release PR; don’t manually bump stable versions as part of the promotion PR.

**Post-release back-merge (main → premain)**
- After the stable release PR merges on `main`, back-merge `main` into `premain` to keep `premain`’s `.release-please-manifest.json` aligned (required by `scripts/verify-branch-version-sync.sh`).
- Keep prerelease version alignment on `premain` (required by `scripts/verify-version-alignment.sh`): TS/Py versions must match `.release-please-manifest.premain.json` on `premain`.
