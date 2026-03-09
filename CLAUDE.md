# t-linear

Go CLI tool for Linear.app. Single-file implementation in `main.go`.

## Build & Install

**Always use `go install .`** — never `go build`. The binary on PATH is at `~/go/bin/t-linear`, not in this directory. `go build` produces a local `.exe` that won't be picked up.

Module path: `github.com/thomas-sievering/t-linear`

## After making changes

1. `go install .`
2. Verify with `t-linear help` or `t-linear version`
