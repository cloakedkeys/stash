# PostgreSQL

Stash keeps its library in an SQLite file by default. It can use a PostgreSQL server instead.

> **⚠️ Note:** PostgreSQL support is new and experimental. SQLite is the better tested choice. There is no way yet to move an existing SQLite library to PostgreSQL, so choose before building a library.

## What the server needs

- PostgreSQL 18 is the version Stash is tested with.
- ICU support, which PostgreSQL packages and the official Docker images include.
- An empty database using the UTF8 encoding, owned by the account Stash connects with. Stash won't start on any other encoding.
- Permission for that account to change the `session_replication_role` setting, which Stash does every time it starts. Superuser accounts already have it, including the one the Docker example below creates.

To create the account and database, run these as a PostgreSQL superuser. On Linux that is usually the `postgres` system user. Elsewhere, add `-U` and the superuser's name to each command, and `-h` and the address for a server on another computer.

```shell
createuser --pwprompt stash
createdb -E UTF8 -T template0 -O stash stash
psql -c "GRANT SET ON PARAMETER session_replication_role TO stash"
```

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
    image: postgres:18
    restart: unless-stopped
    environment:
      POSTGRES_USER: stash
      POSTGRES_PASSWORD: change-me
      POSTGRES_DB: stash
    volumes:
      - ./pgdata:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -U stash -d stash"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 60s
  stash:
    # an image built from this version of Stash; stashapp/stash doesn't support PostgreSQL
    image: stash-local/build:v0.32.1
    restart: unless-stopped
    ports:
      - "9999:9999"
    environment:
      - STASH_DATABASE=postgresql://stash:change-me@db:5432/stash?sslmode=disable
      - STASH_GENERATED=/generated/
      - STASH_METADATA=/metadata/
      - STASH_CACHE=/cache/
      - STASH_PORT=9999
    volumes:
      - ./config:/root/.stash
      - ./data:/data
      - ./metadata:/metadata
      - ./cache:/cache
      - ./blobs:/blobs
      - ./generated:/generated
    depends_on:
      db:
        condition: service_healthy
```

The Stash image is built from this repository with `docker build -t stash-local/build:v0.32.1 -f docker/build/x86_64/Dockerfile .`.

With `STASH_DATABASE` set, Stash skips the setup wizard. After the first start:

1. Set a password for the `admin` account under Settings > Security. Until then, anyone who can reach the port can use Stash.
2. Add your library folder, `/data` in this example, under Settings > Library, then scan.

Keep `STASH_DATABASE` set. The `database` line Stash writes to `config.yml` is the SQLite default, so without the variable Stash would start on an empty SQLite library.

## Regular expressions

Regex filters accept the same [syntax](https://github.com/google/re2/wiki/Syntax) and give the same results on PostgreSQL as on SQLite, including for plugins that use them.

Raw SQL, which some plugins run, goes straight to the database, so it uses PostgreSQL's own [regular expression syntax](https://www.postgresql.org/docs/current/functions-matching.html#FUNCTIONS-POSIX-REGEXP) instead. That includes the `regex_match(text, pattern)` function. A pattern PostgreSQL can't read gives an error saying so. One it can read may still match differently: for example, `.` also matches a new line, and `\w` also matches accented letters.
