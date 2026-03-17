# Session Handoff
**Written:** 2026-03-17

## In Progress
- Nothing — all away-mode tasks completed

## Completed This Session
- M11 fully delivered: RxNorm endpoints, parameterized warmup, multi-instance, nginx LB
  - 6 RxNorm config endpoints validated against live API (17/17 E2E pass)
  - warmup-queries.yaml with top 100 prescribed drugs (196 queries)
  - WarmupOrchestrator wired in main.go with startup warmup
  - ENABLE_SCHEDULER env var for leader/replica mode
  - Nginx least_conn LB on staging (container name `cash-drugs` preserves DNS)
- Staging deployed: 2-instance (leader:8085 + replica:8086) behind nginx LB at :8083
- Cron-based auto-pull replaces Watchtower on staging for all services
- K6 stress-heavy: 95.7% success rate at 150 VUs (up from 21.9% pre-M9)
- Grafana dashboard updated with Instance + Environment variables
- SSH unified: `ssh staging1` across all projects
- Staging deployment guide shared to drug-gate and drugs-quiz
- 23 learnings captured (L-001 through L-023)
- All specs updated (parameterized-warmup, multi-instance → Complete)
- PRD v0.5.0 with M11 DONE, M12 IN_PROGRESS

## Decisions Made
- Multi-instance: ENABLE_SCHEDULER env var (not distributed consensus)
- Nginx least_conn (not round-robin or IP hash) for load balancing
- Staging: 2 instances, production planned for 4
- warmup-queries.yaml from ClinCalc/IQVIA 2023 top 100 prescribed drugs
- Cron auto-pull every 5 min replaces Watchtower

## Blockers
- None

## Next Steps
1. Tag release (v0.9.1 or v1.0.0 — needs human decision on versioning)
2. GA maturity promotion assessment (/add:retro)
3. Production multi-instance deployment (4 instances)
4. Prometheus scrape config for staging leader:8085 + replica:8086
5. M12 remaining: upstream-404-handling (Draft spec, unimplemented)
6. M7 (Auth + Transforms) — last v1 feature
