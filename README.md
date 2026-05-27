# Cooking Helper

A home meal-planning assistant for a family in Finland. One tap generates a weekly menu of
3 recipes (portioned for 7 days) plus a consolidated, store-categorized shopping list. UI in
RU/FI/EN, optimized for an iPad on the kitchen counter.

Architecture is **home-network-first**: data lives on a home server (Mac mini), the iPad is a
thin client on the tailnet. Full context in [`.agents/tech-design.md`](.agents/tech-design.md)
and [`.agents/PRDs/PRD.md`](.agents/PRDs/PRD.md).

> Status: skeleton (CH-2). The app builds, serves `GET /healthz`, and runs in Docker. Data
> models, LLM generation, and the UI land in later stories.

## Prerequisites

- Go 1.24+
- Docker + Docker Compose (for the container workflow)

## Run locally

```bash
go run ./cmd/server
# in another terminal:
curl http://localhost:8080/healthz      # → 200 {"status":"ok"}
```

The server listens on `PORT` (default `8080`) and logs structured JSON to stdout.

## Run with Docker

```bash
docker compose up -d --build
curl http://localhost:8080/healthz      # → 200
docker compose down
```

The SQLite database (added in CH-3) persists in the `cooking-data` volume mounted at `/data`.

## Configuration

| Variable | Required | Purpose |
|----------|----------|---------|
| `PORT` | no (default `8080`) | HTTP listen port. |
| `ANTHROPIC_API_KEY` | not yet | LLM calls (used from a later story). Server-side only — never commit it. Compose reads it from the host environment. |

## Validation

Run before committing:

```bash
gofmt -s -l .            # formatting — no output means clean
go vet ./...
golangci-lint run ./...
go test ./...
```

## HTTPS / PWA

HTTPS is required for the Service Worker and is provided by **Tailscale Serve** on the host
(tailnet-only, automatic Let's Encrypt). That is configured on the Mac mini, not in this repo
(see CH-21 / ops). Plain `go run` over HTTP is fine for local development but will not register
the Service Worker.

## Layout

See [`.agents/tech-design.md`](.agents/tech-design.md) §4.4. Code is grouped by domain feature
under `internal/`, with dependency direction handlers → services → repositories → domain.
