# Maintaining ARTEX Korea

[한국어](MAINTAINING.md) · English

ARTEX Korea defaults to Korean in the UI and newly configured backend output. Builtin prompts, tool descriptions, skills and runtime messages are translated. The earlier global-edition policy of preserving Chinese internal prompts no longer applies to this edition. Historical `work/` records describe earlier decisions.

## Compatibility and upstream changes

Preserve upstream attribution and AGPL-3.0. Keep the Go module path, `artex` binary and Compose service, `ARTEX_*` variables and existing data layout for compatibility. Translate instructions while preserving roles, authorization boundaries, safety constraints, evidence standards, template variables and tool/JSON contracts. Translation checks do not demonstrate equivalent model behavior.

## Migrations

Prompt SHA-256 values identify exact old defaults. Lock the agent, compare its active prompt again, and append a version; keep custom edits and history. Tool migration operates on matching description leaves and preserves schema structure, user defaults, bindings and disabled flags, with a compare-and-swap write. Rule migrations change only exact default display labels. Successful migrations are marked once; later user rollbacks survive restart.

Do not translate stored conversations, reports, raw evidence, custom prompts or explicit language preferences automatically. Chinese catalogs, historical reference documents and legacy parsing/matching tokens remain intentional.

## Validation

Run `go test ./...` and `go vet ./...` with a disposable PostgreSQL database, then frontend tests, `check:i18n`, `check`, `build:static`, and both Python documentation/HTML checks documented in [MAINTAINING.md](MAINTAINING.md). Use Go 1.26.3+ and Node 24.15+ or 22.22.2+. Avoid real credentials and targets. Test migration history, idempotence, custom values and concurrent edits. Keep detection rules and indicator tests aligned when changing audit markers.

## Builds and releases

Compose builds `artex-korea:local` from this checkout. Docker's `local` target builds the frontend and Go binary; `release` consumes CI-built binaries. Secrets and local config are excluded from the build context. Update source builds with `./update.sh` or `./build.sh`.

The release workflow uses the repository it runs in for GHCR and release metadata. Download updates are disabled for unconfigured local builds. Only set build-time `ARTEX_RELEASE_REPO=<owner>/artex-korea` after publishing that actual repository; no upstream/global binary fallback is allowed. Archive and executable names remain `artex` for compatibility.
