# Candidate Managed CLIs

Research into which third-party CLIs Cloak should add as Managed CLIs after the
v1 set (`mongosh`, `psql`, `redis-cli`).

- **Date:** 2026-06-15
- **Status:** Research / prioritization. Not a commitment; no ADR implied.
- **Method:** Cross-referenced 2025–2026 usage data (Stack Overflow Developer
  Survey 2025, JetBrains State of Developer Ecosystem 2025, GitHub Octoverse
  2025, DB-Engines) against a per-CLI audit of native context support. Sources
  are listed at the end.

## Selection filter

A CLI is a good Managed CLI candidate only at the intersection of four
properties:

1. **Widely used** in 2026.
2. **Connection/credential-bearing** — needs a connection target
   (host/endpoint/URI), optional identity, and Secret Material on each
   invocation.
3. **Fits the Connector model** — its connection state can be applied declaratively by
   ephemeral Activation through argv/env, and Explicit Connection Input is
   detectable (per `docs/architecture.md`'s Connector contract).
4. **No native context/profile mechanism.** This is the decisive filter. A CLI
   that already has named, switchable connection profiles gains little from
   Cloak.

The v1 set (`mongosh`, `psql`, `redis-cli`) are all database query clients with
no native context switching. The lowest-risk expansion stays in that shape.

Property 4 eliminates most look-alike candidates — see
[Already context-aware — skip](#already-context-aware--skip).

## Recommended candidates

### Tier 1 — highest priority

| Managed CLI | Backend | 2026 usage | Native context? | Connector fit |
|---|---|---|---|---|
| `mysql` / `mariadb` | MySQL / MariaDB | #2 database, 40.5% (SO 2025) | `mysql`: PARTIAL — `mysql_config_editor` login-paths in `~/.mylogin.cnf` (obscure, no switch command). `mariadb`: NONE | **Strong.** Near-twin of the existing `psql` Connector: `-h -P -u` argv + `MYSQL_PWD` env for Secret Material; positional database = Scope Selection |
| `clickhouse-client` | ClickHouse | Rising fast (leading analytics DB) | PARTIAL — single config file, no named-connection switch | **Strong.** Same shape as `psql`/`mysql`: `--host --port --user --password --secure` |
| `vault` | HashiCorp Vault | High (infra / secrets) | NONE — `VAULT_ADDR` + `VAULT_TOKEN` env only | **Clean** env Activation; token is Secret Material. HashiCorp shipped a separate "Target CLI" specifically to add context switching to Vault/Consul/Nomad — direct evidence of demand |

`mysql`/`mariadb` is the single largest gap: the most-used credential-bearing
CLI not yet covered, and similar declarative bindings to the existing `psql` definition.
Of the other two, `clickhouse-client` is the lowest-risk, on-thesis pick
(another database query client); `vault` has the strongest proven demand but is
a different archetype (address + token rather than host/user/password).

### Tier 2 — strong, more niche or more Connector work

- **`valkey-cli`** (Valkey) — cheapest win: a `redis-cli` fork with identical
  flags, rising as Redis relicensing pushes distros/clouds to Valkey. The
  `redis-cli` Connector nearly is the `valkey-cli` Connector. Secret Material via
  `VALKEYCLI_AUTH`.
- **`cqlsh`** (Cassandra / Scylla) — clean fit: `host port -u -p`, keyspace
  `-k` as Scope Selection. Single `cqlshrc` only (no switching).
- **`cypher-shell`** (Neo4j) — dominant graph database; clean `-a -u -p` fit;
  no native profiles.
- **Kafka — `kcat` + `kafka-console-consumer`/`-producer`** — high operator
  pain and value, but **more Connector work**: the console tools authenticate via
  a `--command-config` properties file, so Activation means generating an
  ephemeral properties file rather than only argv/env. `kcat` is simpler
  (`-b` + `-X sasl.*`).
- **`consul` / `nomad` / `etcdctl`** — bundle with `vault` as the
  HashiCorp/infra set. All env-var Activation (`*_ADDR` / `*_TOKEN`,
  `--endpoints`); none have native contexts.

### Tier 3 — emerging / watch

- **`cockroach sql`** — speaks the PostgreSQL wire protocol via
  `--url postgresql://…`; partly served by a `psql` Connector already.
- **`surreal sql`** (SurrealDB) — clean fit (`--endpoint --user --pass
  --token`), but still early/niche.

## Already context-aware — skip

These look like candidates but already provide named, switchable connection
profiles, so Cloak adds little. Listed because excluding them is the point of
the filter.

| CLI | Native mechanism |
|---|---|
| `sqlcmd` / go-sqlcmd (SQL Server) | `sqlcmd config add-context / use` (kubectl-style). Notable: SQL Server is a top-5 database (~30%), but the modern client already solves this — **do not build it** |
| `influx` (InfluxDB) | `influx config create/set/list` |
| `rclone` | named remotes via `rclone config` |
| `mc` (MinIO) | `mc alias set/list/remove` |
| `rabbitmqadmin` v2 | named node aliases in `~/.rabbitmqadmin.conf` |
| `aws`, `kubectl`, `gcloud`, `az`, `doctl`, `ssh`, `nats`, `snowsql` | profiles / contexts / configurations / Host aliases / `nats context` / named connections (already noted in `README.md`) |

## Not a fit

No connection identity to manage, so the Connector model does not apply:

- **`sqlite`, `duckdb`, `bq`** — local files or GCP application-default
  credentials; no host/identity/secret to activate.
- **Vector / search engines (`qdrant`, `pinecone`, `weaviate`, `meilisearch`)**
  — mostly have no official query CLI to shim; accessed via SDK/REST.

## Connector-design notes

Carry these into any new Connector spec:

1. **`-P` is overloaded.** It means *port* in `mysql`/`mariadb` but *password*
   in some clients (e.g. legacy `sqlcmd`, `snowsql`). Explicit Connection Input
   detection must be per-Connector, never shared across Connectors.
2. **Secret Material passing has ~5 patterns** to model per Connector: inline
   flag (with the tool's own insecurity warning), dedicated env var
   (`MYSQL_PWD`, `REDISCLI_AUTH`, `VAULT_TOKEN`), credential file, connection
   URI, and token-only (vault, etcd). The env-var path is the cleanest
   Activation target and matches the existing `psql` `PGPASSWORD` approach.

## Sources

- [Stack Overflow Developer Survey 2025 — Technology](https://survey.stackoverflow.co/2025/technology/)
- [Stack Overflow 2025 survey — press release](https://stackoverflow.co/company/press/archive/stack-overflow-2025-developer-survey/)
- [DB-Engines ranking](https://db-engines.com/en/ranking)
- [JetBrains State of Developer Ecosystem 2025](https://blog.jetbrains.com/research/2025/10/state-of-developer-ecosystem-2025/)
- [GitHub Octoverse 2025](https://github.blog/news-insights/octoverse/)
- [go-sqlcmd (native contexts)](https://github.com/microsoft/go-sqlcmd)
- [influx config (native contexts)](https://docs.influxdata.com/influxdb/v2/reference/cli/influx/config/)
- [mysql_config_editor / login-paths](https://dev.mysql.com/doc/refman/8.4/en/mysql-config-editor.html)
