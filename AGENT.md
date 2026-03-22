# Agent Notes

- Use `rg` for search and `apply_patch` for edits.
- Avoid destructive git commands.
- Run `go test -count=1 ./...` after code changes.
- Keep production logs minimal; use trace mode only for debugging.
- Treat `cmd/daemon` runtime as the source of truth.
- Keep `focus-events` as the OS activity source.
- Do not reintroduce legacy state-machine code.
- Prefer small, atomic commits.
