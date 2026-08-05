# this justfile lists all the commands that can be run in this project, and how to run them
#

_default:
  @just --choose 

# builds the cli binary and puts it in the bin directory
build:
  go build -ldflags "-X github.com/ceffo/mrboard/internal/cmd/mrboard.Version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o ./bin/mrboard ./cmd/mrboard/...

# runs unit tests for the project
test:
  go test -v ./...

# runs linting on the project using golangci-lint
lint:
  golangci-lint run --allow-parallel-runners --timeout 5m

# formats the code using golangci-lint's fmt command
fmt:
  golangci-lint fmt

# runs all checks for the project required before any commit or pull request
check: fmt lint build test

# run the tui
run: build
  @./bin/mrboard

# run the tui with debug log level 
run-debug: build
  @./bin/mrboard --log-level debug

# fetch calls the fetch command
fetch: build
  @./bin/mrboard fetch

# run the tui against the built-in demo dataset (no config, token, or network)
demo-run: build
  @./scripts/demo-tui.sh

# re-records the README GIF from the working tree — footer reads "…-dirty" (requires vhs)
demo:
  bash scripts/record-demo.sh

# re-records the README GIF from a clean checkout of a tag, so the footer reads
# a real released version rather than "<tag>-N-g<sha>-dirty"
demo-release ref:
  bash scripts/record-demo.sh {{ref}}


# render sample cards to stdout for visual style verification (pipe to a colour-capable terminal)
preview-card:
  go test ./internal/tui/ -run TestCardPreview -v 2>/dev/null

# regenerates all mocks from .mockery.yml (requires mockery v3 and goimports)
generate:
  mockery

# bumps version, tags, and pushes to trigger a release
# no args: interactive prompt (patch|minor|major) with a live version preview
# with args: forwarded as-is, e.g. `just release patch --force`
release *args:
  bash scripts/release.sh {{args}}
