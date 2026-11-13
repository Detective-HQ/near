## Summary

<!-- What does this PR change and why? Link the roadmap/havetoDO phase if relevant. -->

## Changes

-

## Verification

<!-- Paste the commands you ran and their result. CI runs the same checks. -->

- [ ] `cd backend && gofmt -l .` is clean
- [ ] `cd backend && go build ./... && go build -tags tools ./...`
- [ ] `cd backend && go vet ./...`
- [ ] `cd backend && go test -race -count=1 ./...`
- [ ] `cd backend && go run ./cmd/migrate up && go run ./cmd/migrate check`
- [ ] `npm run typecheck`
- [ ] `npm test`
- [ ] `npm run build`

## Checklist

- [ ] Tests cover the new behavior (unit, table, and/or integration)
- [ ] New failures map to a typed `domain.Code`; no raw provider errors leak
- [ ] Migrations are reversible and the drift check passes
- [ ] No secrets, tokens, or private keys added to code, logs, or fixtures
- [ ] Docs updated (`docs/openapi.yaml`, `docs/architecture.md`, `docs/runbook.md`) when behavior changes
- [ ] `havetoDO.md` checkboxes updated when a tracked item is complete
