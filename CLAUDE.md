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
│   └── swagger.*          # OpenAPI/Swagger docs
├── tests/
│   └── e2e/               # End-to-end tests
├── cmd/
│   └── server/            # Application entrypoint
├── internal/
│   ├── cache/             # MongoDB cache layer
│   ├── config/            # YAML config loader
│   ├── handler/           # HTTP handlers
│   ├── upstream/          # Upstream API fetcher
│   ├── scheduler/         # Cron-based refresh
│   ├── fetchlock/         # Dedup concurrent fetches
│   ├── logging/           # Structured logging setup
│   ├── metrics/           # Prometheus metrics & MongoDB collector
│   └── model/             # Response models
├── docker-compose.yml          # Local development
├── docker-compose.prod.yml     # Production (pulls from registry)
├── docker-compose.test.yml     # Test MongoDB
├── Dockerfile                  # Multi-stage alpine build
├── Makefile                    # Test commands
└── go.mod                      # github.com/finish06/cash-drugs
```

### Environments

- **Local:** Docker Compose (http://localhost:8080)
- **Production:** Homelab, pulls from `dockerhub.calebdunn.tech/finish06/cash-drugs`

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
