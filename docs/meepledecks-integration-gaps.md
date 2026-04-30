# Meepledecks integration gaps

This note records gaps found while integrating `github.com/whoisclebs/meepledecks/api` with Golpher.

## Added during integration

- `Request.SetContext(ctx context.Context)` was required so Golpher middleware can attach request-scoped values, such as authenticated user IDs, before downstream handlers run.

## Remaining gaps

- Path-scoped middleware: Meepledecks used Fiber `app.Use("/api/v1/games", ...)`; Golpher currently requires either route-level middleware or a manual path filter.
- Nested groups: Meepledecks used `/api` then `/v1` then feature groups; Golpher currently supports only app-level groups, so integration used flattened prefixes such as `/api/v1/auth`.
- Route pattern metadata: Meepledecks metrics previously labeled by Fiber's route pattern. Golpher does not expose the matched pattern to middleware yet, so the integration temporarily labels metrics by raw path.
- Wildcard routing: Meepledecks previously registered a wildcard middleware path. Golpher's wildcard support needs tests/docs before it should be used for this integration.
- First-party HTTP middleware: request ID, logging and CORS had to be implemented in the application layer.

## Candidate roadmap items

- Add scoped middleware by prefix.
- Add nested `Group.Group(prefix, middlewares...)` support.
- Expose matched route pattern through `Request` or `Context` for observability.
- Complete wildcard matching behavior and tests.
- Provide first-party request ID, logger and CORS middleware packages.
