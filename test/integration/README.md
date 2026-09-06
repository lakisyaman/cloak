# Integration Tests

This package contains Testcontainers-backed integration tests for Cloak's v1 Managed CLI backends:

- PostgreSQL for `psql`
- Redis for `redis-cli`
- MongoDB for `mongosh`

The tests are guarded by the `integration` build tag so normal unit tests stay fast and do not require Docker.

## Run

```bash
go test -tags=integration ./test/integration -count=1 -v
```

If Docker or another Testcontainers provider is not healthy, tests skip through `testcontainers.SkipIfProviderIsNotHealthy`.

## Purpose

These tests verify both layers of Cloak's integration story:

- backend smoke tests prove PostgreSQL, Redis, and MongoDB containers start and respond to their native clients;
- end-to-end Activation tests install a Shim, add a Context, switch the Active Context, and run the real client through Cloak's shim invocation flow against the live backend.

The end-to-end tests assert that each v1 Connector connects, authenticates, and applies its Default Database / logical namespace behavior against a real service.
