# drugs

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

## Commands

### Development
```
docker-compose up                # Start local dev
go run ./cmd/server              # Run server directly
go test ./...                    # Run all tests
go test ./... -short             # Run unit tests only
go test ./... -run Integration   # Run integration tests
go test ./tests/e2e/...          # Run e2e tests
golangci-lint run ./...          # Lint check
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
drugs/
├── .add/                  # ADD methodology config & learnings
├── .claude/               # Claude Code settings & rules
├── specs/                 # Feature specifications
├── docs/
│   ├── prd.md             # Product Requirements Document
│   ├── plans/             # Implementation plans
│   └── milestones/        # Milestone tracking
├── tests/
│   ├── unit/              # Unit tests
│   ├── integration/       # Integration tests
│   └── e2e/               # End-to-end tests
├── cmd/                   # Application entrypoints
├── internal/              # Private application code
├── pkg/                   # Public library code
├── docker-compose.yml     # Local development environment
├── Dockerfile             # Container build
└── go.mod                 # Go module definition
```

### Environments

- **Local:** Docker Compose (http://localhost:8080)
- **Dev:** Homelab (TBD)
- **Staging:** Homelab (TBD)
- **Production:** Homelab (TBD)

## Quality Gates

- **Mode:** Standard
- **Coverage threshold:** 80%
- **Type checking:** go vet (blocking)
- **E2E required:** No

All gates defined in `.add/config.json`. Run `/add:verify` to check.

## Source Control

- **Git host:** GitHub
- **Branching:** Feature branches off `main`
- **Commits:** Conventional commits (feat:, fix:, test:, refactor:, docs:)
- **CI/CD:** GitHub Actions

## Collaboration

- **Autonomy level:** Autonomous
- **Team size:** Small team (2-4)
- **Review gates:** PR review required
- **Deploy approval:** Required for production
