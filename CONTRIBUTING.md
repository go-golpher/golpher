# Contributing

Thanks for helping improve Golpher.

## Development workflow

Golpher is developed with a spec-driven and test-driven workflow:

1. Describe the behavior.
2. Add or update tests.
3. Implement the smallest change that makes tests pass.
4. Refactor while preserving compatibility with `net/http`.

## Local checks

```bash
gofmt -w *.go
go test ./...
go vet ./...
sh scripts/lint.sh
```

## Git hooks

Install local hooks after cloning:

```bash
sh scripts/install-hooks.sh
```

The `commit-msg` hook only accepts Conventional Commits:

```text
<type>(optional-scope): <short summary>
```

Allowed types:

- `feat`
- `fix`
- `docs`
- `refactor`
- `test`
- `perf`
- `chore`
- `ci`
- `build`
- `style`
- `revert`

Examples:

```text
feat(router): add wildcard params
fix(middleware): preserve stdlib response wrappers
ci: add golangci-lint
```

Commit linting is intentionally local-only through the `commit-msg` hook.

## Compatibility rule

Golpher should not trade away `net/http` interoperability for framework-specific convenience. If a change breaks common `http.Handler`, `http.ResponseWriter`, middleware, observability, streaming, or request-cancellation behavior, it needs an explicit design discussion first.
