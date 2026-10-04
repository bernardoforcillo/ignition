# database

Thin Postgres layer over [drops](https://github.com/bernardoforcillo/drops):
a pooled connection, an ordered idempotent migration runner and a transaction
helper. It owns no schema; each service brings its own migrations.

```go
cfg, err := database.FromEnv()          // the only place env is read
db, err := database.Open(ctx, cfg)      // pgx + drops, pings on open
defer db.Close()

//go:embed migrations/*.sql
var migrationFiles embed.FS

migs, err := database.FromFS(migrationFiles, "migrations") // 0001_create_users.sql, ...
err = database.Migrate(ctx, db, migs...)                   // ordered, idempotent, advisory-locked

err = db.WithTx(ctx, func(tx *pg.DB) error { /* queries on tx */ return nil })
```

`DB` embeds `*pg.DB`, so the whole drops query builder, `Ping` (readiness) and
`Close` are available directly. Migrations can also be Go functions
(`Migration{ID, Up}`) or inline SQL (`database.SQL(id, script)`). Each runs in
its own transaction and is recorded in `schema_migrations`; IDs sort
lexicographically, so zero-pad them.

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | Postgres DSN |
| `DATABASE_MAX_OPEN_CONNS` | 10 | pool size |
| `DATABASE_MAX_IDLE_CONNS` | 5 | idle connections (capped at open) |
| `DATABASE_CONN_MAX_LIFETIME` | 30m | Go duration |

## Tests

`go test -short -race ./...` needs no database. DB-backed tests use
`dbtest.Open(t)` and skip unless `TEST_DATABASE_URL` is set:

```sh
TEST_DATABASE_URL=postgres://user:pass@localhost:5432/test go test -race ./...
```
