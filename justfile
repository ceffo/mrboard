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
# `go tool` builds the version go.mod records, so local and CI lint identically
lint:
  go tool golangci-lint run --allow-parallel-runners --timeout 5m

# formats the code using golangci-lint's fmt command
fmt:
  go tool golangci-lint fmt

# reports formatting that fmt would apply, without applying it
fmt-check:
  go tool golangci-lint fmt --diff

# takes the newest release of each dev tool and records it in go.mod
# rerun `just generate` afterwards: a mockery bump changes the generated mocks
tools-update:
  go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
  go get -tool github.com/vektra/mockery/v3@latest
  go mod tidy
  go tool golangci-lint --version

# checks the release scripts' own logic — the PR title → version bump mapping
scripts-test:
  bash scripts/next-version.sh --self-test

# runs all checks for the project required before any commit or pull request
check: fmt lint build test scripts-test

# the CI form of `check`: fails on unformatted code instead of reformatting it
check-ci: fmt-check lint build test scripts-test

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

# regenerates all mocks from .mockery.yml
generate:
  go tool mockery

# bumps version, tags, and pushes to trigger a release
# no args: interactive prompt (patch|minor|major) with a live version preview
# with args: forwarded as-is, e.g. `just release patch --force`
# merging a feat/fix PR releases on its own — this is for a major, or a re-run
release *args:
  bash scripts/release.sh {{args}}

# prints the bump and version merging a PR with this title would release
release-preview title:
  @bash scripts/next-version.sh --plan "{{title}}"
