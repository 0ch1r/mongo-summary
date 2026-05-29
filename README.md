# mongo-summary

`mongo-summary` is a Go command-line tool that reads MongoDB diagnostic text files from a folder and writes a single self-contained HTML summary.

## Usage

```sh
go run ./cmd/mongo-summary \
  -input /path/to/collection-folder \
  -output mongodb-summary.html \
  -title CASE-ID
```

Flags:

- `-input`: folder containing collection artifacts. Defaults to the current directory.
- `-output`: HTML file to write. Defaults to `mongodb-summary.html`.
- `-title`: report title. Defaults to the input folder name.

## Recommended artifact collection format
To avoid parser ambiguity, collect shell outputs as strict JSON (quoted keys + double-quoted strings).

Examples:

```sh
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(db.serverStatus())' > serverStatus.out
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(rs.status())' > rs_status.out
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(rs.conf())' > rs_conf.out
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(db.currentOp())' > currentOp.out
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(db.adminCommand({getParameter:"*"}))' > getParameter.out
mongosh "$MONGO_AUTH" --quiet --eval 'EJSON.stringify(db.adminCommand({getCmdLineOpts:1}))' > getCmdLineOpts.out
```

This tool also normalizes common mongo shell output variants (bare keys, wrapper functions, single-quoted strings), but strict JSON is still preferred for reliability.

## Required Files

Place the following files (with exact names) in the input directory. Timestamp-prefixed variants (e.g., `2026-05-20_20-53-02-serverStatus.out`) and compressed archives (`.tar.gz` / `.tgz`) are **not** supported.

| File | Required? | Source |
|------|-----------|--------|
| `serverStatus.out` | Yes | `mongosh --quiet --eval 'EJSON.stringify(db.serverStatus())'` |
| `rs_status.out` | Yes (for RS) | `mongosh --quiet --eval 'EJSON.stringify(rs.status())'` |
| `rs_conf.out` | Yes (for RS) | `mongosh --quiet --eval 'EJSON.stringify(rs.conf())'` |
| `mongod.log` | Yes | MongoDB server log file |
| `currentOp.out` | Optional | `mongosh --quiet --eval 'EJSON.stringify(db.currentOp())'` |
| `getCmdLineOpts.out` | Optional | `mongosh --quiet --eval 'EJSON.stringify(db.adminCommand({getCmdLineOpts:1}))'` |
| `getParameter.out` | Optional | `mongosh --quiet --eval 'EJSON.stringify(db.adminCommand({getParameter:"*"}))'` |
| `lockinfo.out` | Optional | Raw artifact, included as-is |

Additional files (e.g., `pt-summary.out`, `host_info.out`) will appear in the collection inventory but are not parsed.

## Current Sections

- Collection inventory with file sizes and line counts.
- Server summary from `serverStatus.out`.
- WiredTiger cache and eviction health from `serverStatus.out`.
- Command-line options from `getCmdLineOpts.out`, with fallback extraction from `mongod.log`.
- Replica-set health and a hierarchy view from `rs.status`, `rs.conf`, `printSlaveReplicationInfo`, and `printReplicationInfo`.
- Current operation mix from `currentOp.out`.
- Log overview, severity/message counts, duplicate-key counts, and slow-query namespace summaries from `mongod.log`.
- Top slow query shapes by `queryHash` and `planCacheKey` from `mongod.log`.
- Write concern latency percentiles (P50/P90/P95/P99) for write commands grouped by writeConcern, from `mongod.log` slow-query lines.
- RED summary (Request rate, Error rate, Duration): per-command rates and error percentages from `serverStatus.metrics.commands`, mean latency from `serverStatus.opLatencies`, slow-query P95/P99 joined per command, plus a process-level error pulse from user asserts and log severities. Rates are lifetime averages over server uptime; log-derived rates use the `mongod.log` time window.
- Highlighted runtime parameters from `getParameter.out`.

## Recommended Next Sections

- USE summary (Utilisation, Saturation, Errors) for cache, ticket pools, global lock, connections, flow control, and replication.
- Election and stepdown timeline from replication log events.
- Client IP, driver, and authentication failure breakdowns.
- Sharding and balancer state when config server or `sh.status()` artifacts are available.
