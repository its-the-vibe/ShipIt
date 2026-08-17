# ShipIt

[![CI](https://github.com/its-the-vibe/ShipIt/actions/workflows/ci.yaml/badge.svg)](https://github.com/its-the-vibe/ShipIt/actions/workflows/ci.yaml)

A Go service that consumes GitHub webhook `package` events from a Redis pub/sub channel and dispatches continuous deployment (CD) commands to a Redis list.

## How It Works

1. **Subscribe** – the service subscribes to a configurable Redis pub/sub channel where GitHub webhook `package` events are published.
2. **Filter** – incoming messages are filtered against the following criteria:
   - `action == "published"`
   - `package.package_type == "container"`
   - `package.package_version.container_metadata.tag.name == "latest"`
3. **Whitelist** – the repository full name (`org/repo`) is checked against a configurable allowlist file.
4. **Publish** – matching messages are pushed onto a configurable Redis list as a deployment command.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) & [Docker Compose](https://docs.docker.com/compose/)
- An external Redis instance

## Quick Start

### Local

```bash
# 1. Copy and customise configuration
cp config.example.yaml config.yaml
cp .env.example .env
cp whitelist.txt.example whitelist.txt

# 2. Edit config.yaml to point to your Redis host/port and set channel/list names
# 3. Set REDIS_PASSWORD in .env
# 4. Add repositories to whitelist.txt

# 5. Build and run
make run
```

### Docker

```bash
cp config.example.yaml config.yaml
cp .env.example .env
cp whitelist.txt.example whitelist.txt
# Edit config.yaml, .env, and whitelist.txt

make docker-up
```

## Configuration

| File | Purpose |
|------|---------|
| `config.yaml` | Redis connection, channel/list names, whitelist path (git-ignored) |
| `config.example.yaml` | Template – copy to `config.yaml` |
| `.env` | `REDIS_PASSWORD` secret (git-ignored) |
| `.env.example` | Template – copy to `.env` |
| `whitelist.txt` | Allowed `org/repo` entries (git-ignored) |
| `whitelist.txt.example` | Template – copy to `whitelist.txt` |

### `config.yaml` options

```yaml
redis:
  host: localhost
  port: 6379
  # password is loaded from REDIS_PASSWORD env var

# Redis pub/sub channel to subscribe to
channel: github-webhooks

# Redis list to push deployment commands onto
deploy_list: deployments

# Identifier included in each deployment message as "target-queue"
target_queue: deploy-queue

# Path to the repository whitelist file
whitelist_file: whitelist.txt
```

### `.env` options

```
REDIS_PASSWORD=your_redis_password_here
```

### `whitelist.txt` format

One `org/repo` entry per line; lines starting with `#` and blank lines are ignored:

```
# allowed repositories
your-org/your-repo
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Compile binary to `bin/gdayredis` |
| `make run` | Build and run locally |
| `make test` | Run Go tests |
| `make lint` | Run `go vet` |
| `make docker-build` | Build Docker image |
| `make docker-up` | Start via Docker Compose |
| `make docker-down` | Stop Docker Compose stack |

## Example Webhook Event

The service expects messages on the configured Redis channel to be JSON-encoded GitHub [`package` webhook](https://docs.github.com/en/webhooks/webhook-events-and-payloads#package) payloads.  A minimal triggering example:

```json
{
  "action": "published",
  "package": {
    "package_type": "container",
    "package_version": {
      "container_metadata": {
        "tag": {
          "name": "latest"
        }
      }
    }
  },
  "repository": {
    "full_name": "your-org/your-repo"
  }
}
```

## Deployment Queue Message Format

Messages pushed to the deployment Redis list (`deploy_list`) follow this structure:

```json
{
  "restart": "<org>/<repo>",
  "target-queue": "<target_queue value from config>"
}
```

## Project Layout

```
.
├── cmd/gdayredis/       # Application entry point + tests
├── .github/workflows/
├── config.example.yaml
├── whitelist.txt.example
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod / go.sum
└── README.md
```
