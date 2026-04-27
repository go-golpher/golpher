# Quality

Golpher keeps quality gates close to the codebase so contributors can run the same checks locally and in CI.

## Local commands

```bash
gofmt -w *.go
go test ./...
go test -race ./...
go vet ./...
sh scripts/lint.sh
```

## Linting

Golpher uses `golangci-lint` with configuration in `.golangci.yml`.

Install it from the official docs:

```text
https://golangci-lint.run/welcome/install/
```

Then run:

```bash
golangci-lint run ./...
```

## Conventional Commits

Install the local hook:

```bash
sh scripts/install-hooks.sh
```

Accepted format:

```text
<type>(optional-scope): <short summary>
```

Examples:

```text
feat(router): add path params
fix(middleware): preserve http error responses
docs(readme): add star history
```

## CI checks

GitHub Actions currently run:

- CI: format, vet, lint and tests across Go/OS matrix.
- Lint: dedicated `golangci-lint` workflow.
- Coverage: race test with coverage profile.
- Govulncheck: Go vulnerability scanning.
- CodeQL: GitHub code scanning.

Dependabot keeps Go module metadata and GitHub Actions dependencies up to date.

Commit linting is local-only through `.githooks/commit-msg`; there is no GitHub Actions workflow for commit lint.
