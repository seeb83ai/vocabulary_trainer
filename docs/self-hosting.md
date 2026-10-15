# Self-hosting

The app is a single Go binary with the frontend embedded and a SQLite database. Run it with Docker, natively, or on a Raspberry Pi behind nginx. For environment variables, see [Configuration](configuration.md).

## Docker

**Requirements:** Docker and Docker Compose.

```bash
git clone <repo-url>
cd vocabulary_trainer
make run
```

Then open [http://localhost:8080](http://localhost:8080).

1. Go to **Vocabulary** (`/vocab`) and add some words.
2. Return to **Train** (`/`) to start a quiz session.
3. Check **Mismatches** (`/mismatches`) to review words you confused with each other.

The app stores the SQLite database in `./data/vocab.db` on your host.

## Without Docker

This requires Go 1.25 or later.

```bash
make dev
```

The server listens on `:8080` and stores the database at `data/vocab.db`.

## Deploy to Raspberry Pi

### Initial setup

Locally, copy `.env.example` to `.env` and set `RSYNC_DEST` to configure the deployment target. Note that `make release` syncs only `.env.example`, not `.env`.

```bash
cp .env.example .env
# edit: RSYNC_DEST=pi@raspberrypi.local:/opt/vocab-trainer
```

Run `make release` to copy all needed files.

This target cross-compiles for `linux/arm64` and rsyncs the binary, plus `deploy/nginx.conf` and `deploy/vocab-trainer.service`, to the Pi. Follow the printed instructions to install the systemd service, which auto-restarts when the binary updates, and the nginx reverse proxy.

> If your Pi runs a 32-bit OS, change `GOARCH=arm64` to `GOARCH=arm GOARM=7` in the Makefile.

Copy the `.env.example` file and adjust the settings:

```
cp <deploy-dir>/.env.example <deploy-dir>/.env
```

Then copy or move the service files, and edit them if needed to fix the path and port settings:

```
sudo cp <deploy-dir>/vocab-trainer.service /etc/systemd/system/
sudo cp <deploy-dir>/vocab-trainer-watcher.service /etc/systemd/system/
sudo cp <deploy-dir>/vocab-trainer-watcher.path /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now vocab-trainer
sudo systemctl enable --now vocab-trainer-watcher.path vocab-trainer-watcher.service
sudo systemctl start --now vocab-trainer
sudo systemctl start --now vocab-trainer-watcher.path vocab-trainer-watcher.service
```

To install the nginx config:

```
sudo cp <deploy-dir>/nginx.conf /etc/nginx/sites-available/vocab-trainer
sudo ln -sf /etc/nginx/sites-available/vocab-trainer /etc/nginx/sites-enabled/vocab-trainer
sudo nginx -t && sudo systemctl reload nginx
```

### Release changes

Running `make release` is enough to build the binary, deploy it, and restart the service.

### Backups

The database is a single SQLite file, so backups are simple. The project provides a scheduled nightly backup with retention:

```bash
sudo cp deploy/backup.sh /opt/vocab-trainer/backup.sh
sudo cp deploy/vocab-backup.service /etc/systemd/system/
sudo cp deploy/vocab-backup.timer   /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now vocab-backup.timer
```

`deploy/backup.sh` uses the SQLite online-backup API (`sqlite3 .backup`), which takes a consistent snapshot while the server runs and is safe with WAL mode. It writes timestamped copies to `BACKUP_DIR` (default `data/backups/`) and prunes files older than `RETAIN_DAYS` (default 14 days). The `vocab-backup.timer` runs it nightly.

**Restore.** Stop the server, copy a backup over the live database, and restart:

```bash
sudo systemctl stop vocab-trainer
make restore FROM=data/backups/vocab-2026-06-15_033000.sq3   # or: cp <backup> data/vocab.db
sudo systemctl start vocab-trainer
```

A unit test (`TestBackupRestore_RoundTrip`) covers the backup and restore round-trip, using the same online-backup primitive.

## Upgrade to library references (one-time conversion)

Until this version, a list import copied every word with its translations to the user. On the first start of this version, the server converts these copies once:

1. It fills the shared library with the dictionary translations (log line `Library: filling glosses…`).
2. It writes a backup next to the database: `vocab.db.pre-library-refs` (log line `Library conversion: writing backup …`).
3. Every user word that CC-CEDICT/HanDeDict has becomes a library reference. Users who trained in the last 7 days keep exactly the translations they see now (their own translations stay their own). All other users see the library translations plus their own extra translations. Sentences without a dictionary entry stay unchanged. Progress, tags, mix-ups and mnemonics stay on the same words.
4. The server compares every converted word with the state before. If one differs, it rolls the conversion back and stops with `Library conversion failed and was rolled back`.

Steps on the server:

```bash
sudo systemctl stop vocab-trainer
sqlite3 /opt/vocab-trainer/data/vocab.db ".backup /opt/vocab-trainer/data/vocab.db.before-upgrade"   # optional second backup
# deploy the new binary to /opt/vocab-trainer/vocab-trainer
sudo systemctl start vocab-trainer
journalctl -u vocab-trainer -f    # wait for "Library conversion: N users …"
```

Paths are those of `deploy/vocab-trainer.service`. `make release` copies the new binary into the live directory, and the watcher restarts the service at once, so the conversion starts with the release.

To try the conversion first on a copy (recommended), run the new version against a copy of the database on another port, then compare the result with the backup it wrote:

```bash
sqlite3 /opt/vocab-trainer/data/vocab.db ".backup data/verify.db"   # copy the live DB
cd service && DB_PATH=../data/verify.db PORT=8090 go run .           # log: "Library conversion: N users …"
cd .. && sqlite3 -readonly data/verify.db \
  "ATTACH 'data/verify.db.pre-library-refs' AS old" \
  ".read scripts/verify-library-conversion.sql"
```

The script lists per user how many translations were lost or added. Users who trained in the last 7 days must show `words_changed = 0`, nobody may lose a translation, and the checks for deleted words, changed progress and changed tags must return no rows. To restore after a failed start: stop the server, copy `vocab.db.pre-library-refs` back to `vocab.db`, delete `vocab.db-wal` and `vocab.db-shm`, and deploy the old binary.

## Usage tracking

The app records every request internally, in the `usage_events` table (`user_id`, `name`, `count`, `last_seen`). This lets admins see which pages and endpoints are actually used. The `name` field holds the HTTP method plus the matched route, for example `GET /train` or `POST /api/quiz/answer`. Hits for the same user and route aggregate into one row, with an incrementing `count` and a refreshed `last_seen`. The app records anonymous requests with `user_id = 0`. It excludes static asset requests and audio-streaming routes (`/api/audio/*`, `/api/pinyin-quiz/audio/*`) as noise.

## Admin dashboard

Users with the `admin` role can view cross-user usage insights at `/admin-dashboard` (non-admins are redirected to `/`). Admins land here directly after signing in. It shows:

- **Account overview** — total users, role breakdown (admin/plus/free), email verification status.
- **Activity** — how many registered users trained in the last 7/30 days, and how many are dormant.
- **Signups** — daily registration counts for the last 30 days.
- **Quiz volume** — daily attempts/mistakes summed across all users for the last 30 days.
- **Guest activity** — failed login attempts against unknown or wrong-password accounts (`user_id = 0` in `audit_log`), by day.
- **DeepL / LLM usage** — total calls and unique users for `/api/translate` and the HMM scene-generation endpoints, drawn from `usage_events`.
- **Page views / feature usage** — every tracked route from `usage_events`, split into page navigations and API calls, each with total hits, unique users, and last-seen timestamp.

An "Exclude seed accounts" toggle hides the id=1 (admin/template) and id=2 (operator) accounts from every count and chart, via `GET /api/admin/overview?exclude_seed=1`. It's a UI toggle only — unchecked by default, not persisted.

### Funnel report

The `funnel` CLI tool prints how many users reach each stage of the signup → activation → retention funnel, with conversion percentages. It reads the `users` and `daily_stats` tables and excludes the shared library user (id=1):

```bash
make funnel                          # default: data/vocab.db, engaged = ≥20 attempts
make funnel DB=/path/to/vocab.db MIN_ATTEMPTS=50
```

Stages: **registered** (accounts created) → **verified email** → **activated** (at least one training day) → **engaged** (total attempts reached the threshold), plus **returned** (trained on at least two distinct days) and **returned next day** (trained again the day after the first session). Use it to find the biggest drop-off before investing in features or promotion.
