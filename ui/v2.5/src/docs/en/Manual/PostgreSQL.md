# PostgreSQL

Stash keeps its library in an SQLite file by default. It can use a PostgreSQL server instead.

> **⚠️ Note:** PostgreSQL support is new and experimental. SQLite is the better tested choice. There is no way yet to move an existing SQLite library to PostgreSQL, so choose before building a library.

## What the server needs

- PostgreSQL 17 is the version Stash is tested with.
- The `plperl` extension. On Debian and Ubuntu it is the `postgresql-plperl-17` package. Stash creates the extension itself, so its account must be allowed to: the owner of the database can, since `plperl` is a trusted extension.
- ICU support, which PostgreSQL packages and the official Docker images include.
- An empty database, owned by the account Stash connects with.

Backups and database upgrades also need `pg_dump` and `pg_restore` on the computer that runs Stash, at least as new as the database server. The Stash Docker images include them. Without them, upgrades go ahead with a warning and no backup.

## Choosing PostgreSQL

Stash picks its database once, when it starts, so it can't be changed from the setup wizard. Set the address before the first start, in one of these ways:

- The `STASH_DATABASE` environment variable, for example `STASH_DATABASE=postgresql://stash:password@db-host:5432/stash`. Then start Stash and go through the setup wizard as usual. The wizard's database field is ignored.
- The `database` setting in `config.yml`. A config file written by hand also needs `generated` (where generated files go) and `stash` (the library folders), and then Stash skips the wizard.

The address can start with `postgresql://` or `postgres://`, and takes the usual [connection options](https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING), such as `?sslmode=disable` for a server without TLS.

If Stash skips the wizard, it starts with one `admin` account and no password, and anyone who can reach it can use it. Set a password under Settings > Security before opening it up to others.

## Docker

With Docker Compose, the database and Stash can run side by side:

```yaml
services:
  db:
    image: postgres:17
    environment:
      POSTGRES_USER: stash
      POSTGRES_PASSWORD: change-me
      POSTGRES_DB: stash
    volumes:
      - ./pgdata:/var/lib/postgresql/data
  stash:
    image: stashapp/stash
    environment:
      STASH_DATABASE: postgresql://stash:change-me@db:5432/stash?sslmode=disable
    depends_on:
      - db
```

The official `postgres` image doesn't include `plperl`. Install `postgresql-plperl-17` in it, or build a small image on top of it that does.
