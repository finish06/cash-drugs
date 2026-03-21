# cash-drugs

Go microservice API cache/proxy — queries configurable REST APIs and stores responses in MongoDB for retrieval by other internal microservices.

## Methodology

This project follows **Agent Driven Development (ADD)** — specs drive agents, humans architect and decide, trust-but-verify ensures quality.

- **PRD:** docs/prd.md
- **Specs:** specs/
- **Plans:** docs/plans/
- **Config:** .add/config.json

Document hierarchy: PRD → Spec → Plan → User Test Cases → Automated Tests → Implementation

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Language | Go | 1.22+ |
| HTTP Framework | net/http (stdlib) | — |
| Database | MongoDB | latest |
| Logging | log/slog (stdlib) | — |
| Metrics | Prometheus client_golang | v1.23+ |
| Circuit Breaker | gobreaker | v2.4+ |
| API Docs | swaggo/swag | latest |
| Containers | Docker Compose | — |
| Registry | dockerhub.calebdunn.tech | registry:2 |

## Commands

### Development
```
docker-compose up                # Start local dev
go run ./cmd/server              # Run server directly
make test-unit                   # Run unit tests only
make test-integration            # Run all tests (starts MongoDB via Docker)
make test-coverage               # Run tests with coverage report
go vet ./...                     # Vet check
```

### ADD Workflow
```
/add:spec {feature}                  # Create feature specification
/add:plan specs/{feature}.md         # Create implementation plan
/add:tdd-cycle specs/{feature}.md    # Execute TDD cycle
/add:verify                          # Run quality gates
/add:deploy                          # Commit and deploy
/add:away {duration}                 # Human stepping away
/add:retro                           # Run retrospective
```

## Architecture

### Key Directories
```
cash-drugs/
├── .add/                  # ADD methodology config & learnings
├── .claude/               # Claude Code settings & rules
├── specs/                 # Feature specifications
├── docs/
│   ├── prd.md             # Product Requirements Document
│   ├── plans/             # Implementation plans
│   ├── milestones/        # Milestone tracking (M1-M13+)
│   ├── grafana/           # Grafana dashboard JSON + alerting rules (alerts.yml, alertmanager-template.yml)
│   ├── sequence-diagram.md # Mermaid sequence diagrams for all flows
│   ├── prometheus-setup.md # Prometheus/Grafana monitoring setup
│   ├── staging-deployment.md # Staging environment docs
│   └── swagger.*          # OpenAPI/Swagger docs
├── tests/
│   └── e2e/               # End-to-end tests
├── cmd/
│   └── server/            # Application entrypoint
├── internal/
│   ├── cache/             # MongoDB cache layer + sharded LRU (16-shard FNV-1a)
│   ├── config/            # YAML config loader + staleness/TTL helpers
│   ├── handler/           # HTTP handlers + warmup orchestrator + warmup state tracker + cache status (status.go)
│   ├── upstream/          # Upstream API fetcher + circuit breaker + cooldown + 404 detection + parallel page fetches
│   ├── scheduler/         # Cron-based refresh (endpoints with refresh, no path params)
│   ├── fetchlock/         # Dedup concurrent fetches (sync.Mutex per slug)
│   ├── logging/           # Structured logging setup (slog)
│   ├── metrics/           # Prometheus metrics + MongoDB collector + system collector (procfs)
│   ├── middleware/         # RequestID (outermost, UUID v4) + concurrency limiter (default 50, 503+Retry-After:1) + gzip compression
│   └── model/             # Response models (APIResponse, CachedResponse, ErrorResponse) + error codes (errors.go: CD-H/U/S category codes)
├── docker-compose.yml          # Local development
├── docker-compose.prod.yml     # Production (pulls from registry)
├── docker-compose.test.yml     # Test MongoDB
├── Dockerfile                  # Multi-stage alpine build
├── warmup-queries.yaml              # Parameterized warmup queries (top 100 drugs)
├── Makefile                    # Test commands
└── go.mod                      # github.com/finish06/cash-drugs
```

### Environments

- **Local:** Docker Compose (http://localhost:8080)
- **Staging:** 192.168.1.145:8083 — 2-instance (leader + replica) behind nginx `least_conn` LB. Auto-deploys `:beta` via cron. See `docs/staging-deployment.md`
- **Production:** 192.168.1.86:8083 — pulls from `dockerhub.calebdunn.tech/finish06/cash-drugs:latest`

Multi-instance: `ENABLE_SCHEDULER=true` for leader (runs scheduler + warmup), `ENABLE_SCHEDULER=false` for replicas (serve only).

### CI/CD

- Push to `main` → tests → publish `:beta` image
- Push git tag `v*` → tests → publish `:vX.Y.Z` + `:latest`
- Registry: `dockerhub.calebdunn.tech/finish06/cash-drugs`

## Deploy Expectations

When deploying changes that modify routes, handlers, middleware, or the upstream integration:
- Update `docs/sequence-diagram.md` to reflect the current request flows
- Ensure new endpoints, error paths, and middleware are represented in the Mermaid diagrams

## Quality Gates

- **Mode:** Standard
- **Coverage threshold:** 80%
- **Type checking:** go vet (blocking)
- **E2E required:** No

All gates defined in `.add/config.json`. Run `/add:verify` to check.

## Source Control

- **Repo:** github.com/finish06/cash-drugs
- **Branching:** Feature branches off `main`
- **Commits:** Conventional commits (feat:, fix:, test:, refactor:, docs:)
- **CI/CD:** GitHub Actions

## Collaboration

- **Autonomy level:** Autonomous
- **Team size:** Small team (2-4)
- **Review gates:** PR review required
- **Deploy approval:** Required for production
