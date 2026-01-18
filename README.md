# Greeder

A Go reimplementation of SpeedyReader: a terminal RSS reader with AI summaries, OPML support, and Raindrop.io bookmarking. This version uses local LLMs via an OpenAI-compatible API.

## Features

- Feed discovery from a site URL (RSS or Atom)
- Charmbracelet-based TUI with tooltips and a `/` quick-reference popover
- Article list with read/star flags and filters
- AI summaries from a local OpenAI-compatible endpoint
- OPML import/export
- Raindrop.io bookmarking with summary notes
- Open in browser and email share shortcuts
- Local JSON storage with 7-day cleanup on startup

## Installation

Requires Go 1.20+.

```bash
go build -o greeder .
```

## Configuration

A config file is created on first run at `~/.config/speedy-reader/config.toml`.

```toml
db_path = "/path/to/feeds.json"
refresh_interval_minutes = 30
default_tags = ["rss"]
raindrop_token = "..." # optional
```

Notes:
- `db_path` stores a JSON file (not SQLite).
- `raindrop_token` enables bookmarking.

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
```

### Key bindings

| Command | Action |
| --- | --- |
| `j` / `down` | Move down |
| `k` / `up` | Move up |
| `enter` | Generate/show summary |
| `r` / `refresh` | Refresh feeds |
| `a <url>` / `add <url>` | Add feed |
| `i <path>` / `import <path>` | Import OPML |
| `w <path>` / `export <path>` | Export OPML |
| `s` / `star` | Toggle starred |
| `m` / `mark` | Toggle read/unread |
| `o` / `open` | Open in browser |
| `e` / `email` | Email article |
| `b <tag,tag>` / `bookmark <tag,tag>` | Save to Raindrop |
| `f` / `filter` | Cycle filter (Unread/Starred/All) |
| `d` / `delete` | Delete article |
| `u` / `undelete` | Restore last deleted |
| `/` | Toggle quick command reference |
| `q` / `quit` | Quit |

## Data storage

Feeds, articles, summaries, and Raindrop state are stored in the JSON file configured by `db_path`.

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
