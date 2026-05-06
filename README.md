# SpotSonic

[![CI](https://github.com/justsky/spotsonic/actions/workflows/ci.yml/badge.svg)](https://github.com/justsky/spotsonic/actions/workflows/ci.yml)
[![Release](https://github.com/justsky/spotsonic/actions/workflows/release.yml/badge.svg)](https://github.com/justsky/spotsonic/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/justsky/spotsonic)](https://goreportcard.com/report/github.com/justsky/spotsonic)
[![Docs](https://img.shields.io/badge/docs-justsky.github.io%2Fspotsonic-green)](https://justsky.github.io/spotsonic)

Convert [Exportify](https://github.com/watsonbox/exportify) CSV playlists into Navidrome playlists via the Subsonic API, with incremental weekly updates.

## How it works

**First run** — creates playlists:
1. Reads Exportify CSV files
2. Searches your Navidrome library via the Subsonic `search3` API
3. Fuzzy-matches by title (60%) + artist (40%)
4. Creates playlists in Navidrome; saves state to `spotsonic-state.json`

**Subsequent runs** — updates playlists:
1. Loads state from previous run
2. Retries previously unmatched tracks (songs added to your library since last run)
3. Detects new tracks added to CSV (re-exported Exportify playlist)
4. Appends newly matched songs to existing Navidrome playlists
5. If a playlist was deleted in Navidrome, recreates it automatically

## Requirements

- Go 1.22+
- A running [Navidrome](https://www.navidrome.org/) instance
- Exportify CSV exports from [Exportify](https://github.com/watsonbox/exportify)

## Installation

```bash
git clone https://github.com/justsky/spotsonic
cd spotsonic
go build -o spotsonic .
```

## Usage

```
spotsonic [flags]

Flags:
  -server     string   Navidrome URL (e.g. http://localhost:4533)          [required]
  -user       string   Navidrome username                                  [required]
  -password   string   Navidrome password                                  [required]
  -input      string   Input CSV file or directory (default ".")
  -state      string   State file path (default "spotsonic-state.json")
  -threshold  float    Fuzzy match threshold 0.0–1.0 (default 0.80)
  -dry-run             Preview matches without creating or modifying playlists
  -report     string   Write currently unmatched tracks to this CSV file
  -version             Print version and exit
```

### Examples

**First run — create all playlists:**
```bash
spotsonic \
  -server http://localhost:4533 \
  -user admin \
  -password secret \
  -input ./spotify_playlists/ \
  -state spotsonic-state.json \
  -report unmatched.csv
```

**Subsequent runs — retry unmatched, add newly found songs:**
```bash
spotsonic \
  -server http://localhost:4533 \
  -user admin \
  -password secret \
  -input ./spotify_playlists/ \
  -state spotsonic-state.json \
  -report unmatched.csv
```

The command is identical — SpotSonic automatically detects whether a playlist is new (creates it) or already exists (updates it) based on the state file.

**Dry-run preview:**
```bash
spotsonic \
  -server http://localhost:4533 \
  -user admin \
  -password secret \
  -input my_playlist.csv \
  -dry-run
```

## Weekly Scheduling

### WSL / Linux (cron)

1. Edit `scripts/run.sh` and fill in your Navidrome credentials (or set them as environment variables).

2. Install the weekly cron job:
   ```bash
   bash scripts/setup_cron.sh
   ```
   This installs a job that runs every Monday at 09:00.

3. Edit the crontab to set your password:
   ```bash
   crontab -e
   ```

4. Enable the cron service in WSL if needed:
   ```bash
   sudo service cron start
   # To start automatically on WSL launch, add the above to ~/.bashrc or /etc/wsl.conf
   ```

### Windows Task Scheduler

Run SpotSonic through WSL on a weekly schedule:

```powershell
$action = New-ScheduledTaskAction `
  -Execute "wsl.exe" `
  -Argument "-e bash /mnt/d/Github/SpotSonic/scripts/run.sh"

$trigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday -At 9am

Register-ScheduledTask `
  -TaskName "SpotSonic Weekly" `
  -Action $action `
  -Trigger $trigger `
  -RunLevel Highest
```

Set `NAVIDROME_PASSWORD` in `scripts/run.sh` or pass it via the task's environment.

## State File

`spotsonic-state.json` is the key to incremental updates. It tracks:
- Navidrome playlist ID per CSV (so songs are appended to the right playlist)
- Spotify URI → Navidrome song ID for every matched track (prevents duplicates)
- Full details of every unmatched track (retried on next run)

Keep the state file alongside your CSV files. If lost, the next run recreates all playlists from scratch.

The state file is gitignored by default. Do not commit it.

## CSV Format

SpotSonic expects the default [Exportify](https://github.com/watsonbox/exportify) export with at least:

| Column | Used for |
|--------|----------|
| `Track URI` | Unique identifier (deduplication across runs) |
| `Track Name` | Search query + match scoring |
| `Album Name` | Displayed in logs |
| `Artist Name(s)` | Match scoring (primary artist) |

All other Exportify columns (audio features, popularity, etc.) are ignored.

## Matching

Tracks are matched in two passes:

1. Search by **title** → score candidates by title (60%) + artist (40%)
2. If no match: search by **"artist title"** combined → rescore

A match is accepted when the combined score meets `-threshold` (default 0.80 = 80%).

**Adjust the threshold** to tune precision vs. recall:
- Higher (e.g. `0.90`) → fewer false positives, more unmatched
- Lower (e.g. `0.70`) → more matches, possible false positives

## Playlist Names

Derived from CSV filenames — underscores become spaces:
- `My_Playlist.csv` → `My Playlist`
- `505_vibes_but_better.csv` → `505 vibes but better`

## Unmatched Report

With `-report unmatched.csv`, all currently unmatched tracks are written each run:

```
Playlist,Track Name,Artist,Album
My Playlist,Some Song,Some Artist,Some Album
```

Tracks listed here will be retried automatically on the next run. Use this report to identify songs genuinely missing from your local library.

## License

MIT
