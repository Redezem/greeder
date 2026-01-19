# Greeder

A Go reimplementation of SpeedyReader: a terminal RSS reader with AI summaries, OPML support, and Raindrop.io bookmarking. This version uses local LLMs via an OpenAI-compatible API.

## Features

- Feed discovery from a site URL (RSS or Atom)
- Charmbracelet-based TUI with tooltips and a `/` quick-reference popover
- Article list with read/star flags, filters, and summary spinners
- AI summaries from a local OpenAI-compatible endpoint (async + batch)
- Concurrent feed refresh with status spinner
- Split detail view with metadata (published time, feed, author, URL)
- Copy article URLs to clipboard
- OPML import/export
- Export/import subscriptions plus article state
- Raindrop.io bookmarking with summary notes
- Open in browser and email share shortcuts
- SQLite storage with 7-day cleanup on startup

## Installation

Requires Go 1.22+.

```bash
go build -o greeder .
```

Clipboard support on Linux requires `wl-copy` (Wayland) or `xclip`/`xsel` (X11).

## Configuration

A config file is created on first run at `~/.config/greeder/config.toml`.

```toml
db_path = "/path/to/feeds.db"
refresh_interval_minutes = 30
default_tags = ["rss"]
raindrop_token = "..." # optional
```

Notes:
- `db_path` stores a SQLite database.
- Default data path is `~/.local/share/greeder/feeds.db` (or `XDG_DATA_HOME/greeder/feeds.db`).
- `raindrop_token` enables bookmarking.

## Migration

If the Greeder config does not exist but legacy SpeedyReader files are found, Greeder offers a one-time migration to copy the config and import the JSON database into SQLite.

## Local LLM setup

Set these environment variables to enable summaries:

- `LM_BASE_URL` (required): base URL for your OpenAI-compatible server, for example `http://localhost:8080` or `http://localhost:8080/v1`
- `LM_API_KEY` (optional): API key if your server requires one
- `LM_MODEL` (optional): model name, default `gpt-4o-mini`

## Usage

```bash
# Run the interactive TUI
./greeder

# Import OPML
./greeder --import feeds.opml

# Refresh feeds headlessly
./greeder --refresh

# Export/import state (feeds + articles + summaries)
./greeder --export-state state.json
./greeder --import-state state.json
```

### Key bindings

| Command | Action |
| --- | --- |
| `j` / `down` | Move down |
| `k` / `up` | Move up |
| `enter` | Generate/show summary |
| `G` | Generate summaries for all missing articles |
| `r` / `refresh` | Refresh feeds |
| `a <url>` / `add <url>` | Add feed |
| `i <path>` / `import <path>` | Import OPML |
| `w <path>` / `export <path>` | Export OPML |
| `I <path>` / `import-state <path>` | Import state |
| `E <path>` / `export-state <path>` | Export state |
| `s` / `star` | Toggle starred |
| `m` / `mark` | Toggle read/unread |
| `o` / `open` | Open in browser |
| `e` / `email` | Email article |
| `y` / `copy` | Copy article URL to clipboard |
| `pgup`/`pgdn` or `ctrl+u`/`ctrl+d` | Scroll detail pane |
| `b <tag,tag>` / `bookmark <tag,tag>` | Save to Raindrop |
| `f` / `filter` | Cycle filter (Unread/Starred/All) |
| `d` / `delete` | Delete article |
| `u` / `undelete` | Restore last deleted |
| `/` | Toggle quick command reference |
| `q` / `quit` | Quit |

## Data storage

Feeds, articles, summaries, and Raindrop state are stored in the SQLite database configured by `db_path`.

## State export/import

Use `--export-state` and `--import-state` to move subscriptions and article state between machines. This exports feeds, articles, summaries, saved bookmarks, and deleted entries to a JSON file.

## Tests

```bash
go test ./...

go test -cover ./...

go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## Taskfile

If you use [Task](https://taskfile.dev), you can run common workflows:

```bash
task build
task run
task test
task test:cover
task fmt
task lint
task tidy
```

## License

MIT
