# this justfile lists all the commands that can be run in this project, and how to run them
#

_default:
  @just --choose 

# builds the cli binary and puts it in the bin directory
build:
  go build  -o ./bin/mrboard ./cmd/cli/...

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


