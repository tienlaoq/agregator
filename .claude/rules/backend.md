---
paths:
  - "services/**/*.go"
  - "pkg/**/*.go"
---

# Go Backend Rules

- Accept interfaces, return concrete types; define interfaces at usage site, not implementation site
- Wrap errors with `fmt.Errorf("context: %w", err)`; never ignore errors; custom domain errors in `internal/domain/`
- Pass `context.Context` as first param on every function that touches I/O or calls other services
- Table-driven tests with `t.Run`; mock via interface fields (no third-party mock libs); always run with `-race`
- `gofmt` + `goimports` before committing; exactly one package declaration per file
- Use `sync.WaitGroup` / channels for goroutine coordination; always provide an exit path (no goroutine leaks)
- Repository layer: parameterized SQL only — no string interpolation
- gRPC delivery layer: map domain errors to `status.Errorf(codes.X, ...)` — never expose internals to callers
