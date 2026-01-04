# Developer Workflow

This project standardises its developer tasks around Mage to keep the Clean Architecture codebase reproducible across environments.

## Mage Targets

The root `magefile.go` exposes the following commands:

- `mage format` – runs `go fmt ./...` over the repository.
- `mage lint` – executes `go vet ./...`.
- `mage test` – runs the full test suite (`go test ./...`).
- `mage build` – compiles all packages (`go build ./...`) and drops the `cr` binary in the repository root with version metadata baked in.
- `mage ci` – the default target; chains format, lint, test, and build in that order.

Running `mage` with no arguments executes `mage ci`.

## CLI Versioning

The CLI exposes `-v` / `--version` flags on every command. `mage build` injects the most recent Git tag as the version; if the working tree or HEAD diverges from that tag, the string is suffixed with `-dirty`. When no tags are present, the default version is `v0.0.0`.

To review uncommitted work on the checked-out branch, run `bop review branch --include-uncommitted`; the command auto-detects the current branch unless you pass a positional name or `--target`. The CLI still expects `git` to be available so it can diff the working tree against the chosen base reference.
