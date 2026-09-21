# Integration Tests

This package contains Testcontainers-backed integration tests for Cloak's v1 Managed CLI backends:

- PostgreSQL for `psql`
- MySQL for `mysql`
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

The MySQL DNS SRV regression uses two MySQL 8.4 servers and a CoreDNS container.
It verifies that the real client reaches the second server through DNS, that a
wrong password is rejected, and that an invocation through Cloak does not use
credentials saved for the first server. Server UUIDs identify which server
authenticated the client. Cloak runs in-process through the existing integration
delegate; the MySQL client and servers are real binaries in containers.

Run this regression alone with:

```bash
go test -tags=integration ./test/integration -run '^TestMySQLDNSSRVDoesNotReceiveSavedCredentials$' -count=1 -v
```
