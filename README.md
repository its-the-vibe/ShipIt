# ShipIt

[![CI](https://github.com/its-the-vibe/ShipIt/actions/workflows/ci.yaml/badge.svg)](https://github.com/its-the-vibe/ShipIt/actions/workflows/ci.yaml)

A Go service that consumes GitHub webhook `package` events (and optionally custom Docker image push events) from Redis pub/sub channels and dispatches continuous deployment (CD) commands to a Redis list.

## How It Works

1. **Subscribe** – the service subscribes to a configurable Redis pub/sub channel where GitHub webhook `package` events are published.  Optionally, it also subscribes to a separate channel for custom Docker image push payloads.
2. **Filter** – incoming GitHub `package` messages are filtered against the following criteria:
   - `action == "published"`
   - `package.package_type == "CONTAINER"`
   - `package.package_version.container_metadata.tag.name == "latest"`

   Incoming custom image push messages are filtered against:
   - `event == "image_pushed"`
   - `ref == "main"`
3. **Whitelist** – the repository full name (`org/repo`) is checked against an allowlist defined in `config.yaml`.
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

# 2. Edit config.yaml: set Redis host/port, channel/list names, and whitelist repos
# 3. Set REDIS_PASSWORD in .env

# 4. Build and run
make run
```

### Docker

```bash
cp config.example.yaml config.yaml
cp .env.example .env
# Edit config.yaml and .env

make docker-up
```

## Configuration

| File | Purpose |
|------|---------|
| `config.yaml` | Redis connection, channel/list names, and whitelist (git-ignored) |
| `config.example.yaml` | Template – copy to `config.yaml` |
| `.env` | `REDIS_PASSWORD` secret (git-ignored) |
| `.env.example` | Template – copy to `.env` |

### `config.yaml` options

```yaml
redis:
  host: localhost
  port: 6379
  # password is loaded from REDIS_PASSWORD env var

# Redis pub/sub channel to subscribe to
channel: github-webhooks

# Optional: Redis pub/sub channel for custom Docker image push payloads
# custom_channel: custom-image-pushes

# Redis list to push deployment commands onto
deploy_list: deployments

# Identifier included in each deployment message as "target-queue"
target_queue: deploy-queue

# Repositories allowed to trigger deployments
whitelist:
  - your-org/your-repo
```

### `.env` options

```
REDIS_PASSWORD=your_redis_password_here
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Compile binary to `bin/shipit` |
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

## Custom Image Push Payload

When `custom_channel` is configured, the service also accepts custom Docker image push payloads on that channel.  A minimal triggering example:

```json
{
  "event": "image_pushed",
  "repository": "your-org/your-repo",
  "ref": "main",
  "sha": "abc1234",
  "image": "ghcr.io/your-org/your-repo",
  "tags": ["latest"]
}
```

**Filtering rules for custom payloads:**
- `event` must be `"image_pushed"`
- `ref` must be `"main"`
- `repository` must be present in the `whitelist`

## Deployment Queue Message Format

Messages pushed to the deployment Redis list (`deploy_list`) follow this structure (identical for both payload types):

```json
{
  "restart": "<repo>",
  "target-queue": "<target_queue value from config>"
}
```

## Project Layout

```
.
├── cmd/shipit/          # Application entry point + tests
├── .github/workflows/
├── config.example.yaml
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod / go.sum
└── README.md
```
