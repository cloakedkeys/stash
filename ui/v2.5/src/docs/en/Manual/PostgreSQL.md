# PostgreSQL

Stash keeps its library in an SQLite file by default. It can use a PostgreSQL server instead.

> **⚠️ Note:** PostgreSQL support is new and experimental. SQLite is the better tested choice. An existing SQLite library can be moved to PostgreSQL, as explained below, but there's no way to move back.

## What the server needs

- PostgreSQL 18 is the version Stash is tested with.
- ICU support, which PostgreSQL packages and the official Docker images include.
- An empty database using the UTF8 encoding, owned by the account Stash connects with. Stash won't start on any other encoding.

The account doesn't need to be a superuser. To create the account and database, run these as a PostgreSQL superuser. On Linux that is usually the `postgres` system user. Elsewhere, add `-U` and the superuser's name to each command, and `-h` and the address for a server on another computer.

```shell
createuser --pwprompt stash
createdb -E UTF8 -T template0 -O stash stash
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
      - STASH_STASH=/data/
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
2. Run a scan. Your library folder comes from `STASH_STASH`, `/data/` in this example. Once you save folders under Settings > Library, those are used instead.

Keep `STASH_DATABASE` set. The `database` line Stash writes to `config.yml` is the SQLite default, so without the variable Stash would start on an empty SQLite library.

## Moving a SQLite library to PostgreSQL

Stash can copy a SQLite library into an empty PostgreSQL database. Everything in the database comes across, including accounts and their ratings and play history, with the same IDs. Your SQLite file isn't changed, so you can go back to it, but changes made after the copy stay in PostgreSQL. There's no way to copy from PostgreSQL to SQLite.

1. Start this version of Stash on SQLite once, so the library is upgraded if it needs to be, then stop Stash. Changes made while the copy runs wouldn't be included.
2. Set up the PostgreSQL server as described in "What the server needs". If Stash has already started on that database, it's fine as long as there's no library in it yet.

   The copy needs one extra permission unless the account is a superuser, which includes the one the Docker example creates. Run this as a superuser first, and you can take the permission away again afterwards with `REVOKE`:

   ```shell
   psql -c "GRANT SET ON PARAMETER session_replication_role TO stash"
   ```

3. Set `STASH_DATABASE` to the address of the PostgreSQL database and run the copy, giving it the SQLite file:

   ```shell
   stash --migrate-sqlite-to-postgres /path/to/stash-go.sqlite
   ```

   With the Docker example above, once the `stash` service has `STASH_DATABASE` set:

   ```shell
   docker compose run --rm stash --migrate-sqlite-to-postgres /root/.stash/stash-go.sqlite
   ```

4. Keep `STASH_DATABASE` set, and start Stash as usual.

The copy happens all at once. If it fails, the PostgreSQL database stays empty, and it can run again. It only copies into an empty database, so it can't overwrite a library.

- **Values PostgreSQL can't store:** SQLite accepts things PostgreSQL won't, such as a file path that isn't valid UTF-8 or an impossible date. The copy stops and lists each one by table and row, with nothing copied, so they can be fixed first.
- **Files outside the database:** generated files, and images if blobs are kept on the filesystem, stay where they are. Stash finds them the same way, as long as the settings still point to them.
- **Regex filters and raw SQL:** plugins that use raw SQL may need changes, because SQLite and PostgreSQL differ. Tables a plugin added to the SQLite database aren't copied, and the copy names them.

## Upgrading to a new PostgreSQL version

PostgreSQL 19 isn't out yet. These steps were tested going from 18.6 to 19 RC1. Wait for the final release, planned for 19 November 2026, before doing this on a library you care about. PostgreSQL 18 is still the version Stash is tested with.

A new major version can't open an older version's data folder. `pg_upgrade` converts it.

Stash's backups use `pg_dump`, which must be at least as new as the server. The Stash Docker images use Alpine's PostgreSQL client, which is still 18, with no 19 package yet. On a PostgreSQL 19 server, those images can't make backups until they have a 19 client. Database upgrades go ahead with a warning and no backup, as described in "What the server needs". You may want to wait until the images have a 19 client.

With the Docker example above, run these commands from the folder holding `docker-compose.yml`:

1. Stop everything and keep a copy of the data. The copy is the way back:

   ```shell
   docker compose down
   sudo cp -a pgdata pgdata-backup
   ```

2. Build an image holding both versions. Save this as `Dockerfile.pg-upgrade`:

   ```dockerfile
   FROM postgres:19
   RUN apt-get update && apt-get install -y --no-install-recommends postgresql-18
   ```

   ```shell
   docker build -t stash-pg-upgrade -f Dockerfile.pg-upgrade .
   ```

3. Save this as `pg-upgrade.sh`:

   ```bash
   #!/bin/bash
   set -euo pipefail
   OLD=/var/lib/postgresql/18/docker
   NEW=/var/lib/postgresql/19/docker
   B18=/usr/lib/postgresql/18/bin
   B19=/usr/lib/postgresql/19/bin
   mkdir -p "$NEW"
   chown -R postgres:postgres /var/lib/postgresql/19
   chmod 700 "$NEW"
   cd /var/lib/postgresql/19
   gosu postgres "$B19/initdb" -D "$NEW" -U "$SUPERUSER" -E UTF8 --locale=en_US.utf8
   gosu postgres "$B19/pg_upgrade" -b "$B18" -B "$B19" -d "$OLD" -D "$NEW" -U "$SUPERUSER" --link --check
   gosu postgres "$B19/pg_upgrade" -b "$B18" -B "$B19" -d "$OLD" -D "$NEW" -U "$SUPERUSER" --link
   cp "$OLD/pg_hba.conf" "$NEW/pg_hba.conf"
   chown postgres:postgres "$NEW/pg_hba.conf"
   ```

   Then run it. `SUPERUSER` is the `POSTGRES_USER` from the Compose file, `stash` in the example:

   ```shell
   docker run --rm -e SUPERUSER=stash -v "$PWD/pgdata:/var/lib/postgresql" -v "$PWD/pg-upgrade.sh:/pg-upgrade.sh:ro" stash-pg-upgrade bash /pg-upgrade.sh
   ```

   The `--check` line stops before anything has changed if the data can't be upgraded.

4. In `docker-compose.yml`, change the database image to `postgres:19`. It finds its data in `19/docker` by itself. Start the database and rebuild its statistics, then start the rest:

   ```shell
   docker compose up -d db
   docker compose exec db vacuumdb -U stash --all --analyze-in-stages --missing-stats-only
   docker compose exec db vacuumdb -U stash --all --analyze-only
   docker compose up -d
   ```

5. Once Stash looks right, remove the old data and the copy. This needs root because the `18` folder belongs to root, so `pg_upgrade`'s own `delete_old_cluster.sh` can't finish it:

   ```shell
   docker run --rm -v "$PWD/pgdata:/var/lib/postgresql" stash-pg-upgrade rm -rf /var/lib/postgresql/18
   sudo rm -rf pgdata-backup
   ```

- **Space and the way back:** `--link` is quick and uses no extra space, but after the new server has started, the old folder can't be used. That is why step 1 keeps a copy. `pgdata` must be on one filesystem. Removing `--link` from both lines copies the data instead, which is slower and needs room for a second copy. The `--link` route was tested; the copying route wasn't.
- **An ordinary account:** if Stash connects with an ordinary account as described in "What the server needs", `SUPERUSER` and `-U` are the actual superuser, often `postgres`, instead of `stash`. The account, its password, its permissions and the `session_replication_role` grant all carry over and do not need redoing. This was tested.
- **A server outside Docker:** use the same `pg_upgrade` steps with your system's paths, and see PostgreSQL's [`pg_upgrade` documentation](https://www.postgresql.org/docs/current/pgupgrade.html). This wasn't tested.

## Regular expressions

Regex filters accept the same [syntax](https://github.com/google/re2/wiki/Syntax) and give the same results on PostgreSQL as on SQLite, including for plugins that use them.

Raw SQL, which some plugins run, goes straight to the database, so it uses PostgreSQL's own [regular expression syntax](https://www.postgresql.org/docs/current/functions-matching.html#FUNCTIONS-POSIX-REGEXP) instead. That includes the `regex_match(text, pattern)` function. A pattern PostgreSQL can't read gives an error saying so. One it can read may still match differently: for example, `.` also matches a new line, and `\w` also matches accented letters.
