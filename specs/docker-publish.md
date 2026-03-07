# Spec: Docker Build & Publish

**Version:** 0.1.0
**Created:** 2026-03-07
**PRD Reference:** docs/prd.md
**Status:** Draft

## 1. Overview

Build and publish the drugs Docker image to a private self-hosted registry (`dockerhub.calebdunn.tech`) with versioned tags. Every merge to `main` publishes a `:beta` image for testing. Pushing a git version tag (e.g., `v0.5.0`) publishes `:v0.5.0` and `:latest`. The Go binary embeds build version via `-ldflags` so running containers report their version at runtime.

### User Story

As a developer deploying drugs to my homelab, I want Docker images automatically built and pushed to my private registry with clear version tags, so that I can pull specific versions or latest for deployment without manual builds.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | Dockerfile builds a linux/amd64 image with the Go binary | Must |
| AC-002 | Go binary has version embedded via `-ldflags -X main.version={tag}` at build time | Must |
| AC-003 | `GET /health` response includes a `version` field showing the embedded build version | Must |
| AC-004 | CI workflow pushes `dockerhub.calebdunn.tech/finish06/cash-drugs:beta` on every push to `main` | Must |
| AC-005 | CI workflow pushes `dockerhub.calebdunn.tech/finish06/cash-drugs:v{X.Y.Z}` when a git tag `v*` is pushed | Must |
| AC-006 | CI workflow pushes `dockerhub.calebdunn.tech/finish06/cash-drugs:latest` when a git tag `v*` is pushed | Must |
| AC-007 | CI authenticates to the registry using GitHub Actions secrets `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` | Must |
| AC-008 | Existing CI test job still runs and must pass before any image is published | Must |
| AC-009 | Image is built as a multi-stage build (existing Dockerfile pattern) keeping final image small (alpine-based) | Must |
| AC-010 | Build target is explicitly `linux/amd64` | Must |
| AC-011 | Docker build and push only runs after tests pass (job dependency) | Should |
| AC-012 | Failed push to registry does not mark the test job as failed (separate job) | Should |

## 3. User Test Cases

### TC-001: Beta image published on merge to main

**Precondition:** CI secrets `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` configured in GitHub repo settings
**Steps:**
1. Push a commit or merge a PR to `main`
2. CI test job runs and passes
3. CI publish job builds Docker image
4. CI pushes image to `dockerhub.calebdunn.tech/finish06/cash-drugs:beta`
**Expected Result:** Image is pullable from registry with `:beta` tag; running container reports version as the git SHA or branch ref
**Screenshot Checkpoint:** N/A (CI pipeline)
**Maps to:** TBD

### TC-002: Versioned image published on git tag

**Precondition:** CI secrets configured, code on `main` is passing
**Steps:**
1. Tag the current commit: `git tag v0.5.0 && git push origin v0.5.0`
2. CI test job runs and passes
3. CI publish job builds Docker image
4. CI pushes image with tags `:v0.5.0` and `:latest`
**Expected Result:** Both `dockerhub.calebdunn.tech/finish06/cash-drugs:v0.5.0` and `:latest` are pullable; container `/health` returns `"version": "v0.5.0"`
**Screenshot Checkpoint:** N/A (CI pipeline)
**Maps to:** TBD

### TC-003: Version visible at runtime

**Precondition:** Container running from a tagged image
**Steps:**
1. `docker run dockerhub.calebdunn.tech/finish06/cash-drugs:v0.5.0`
2. `curl http://localhost:8080/health`
**Expected Result:** Response JSON includes `"version": "v0.5.0"`
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

### TC-004: Publish blocked by failing tests

**Precondition:** A commit with a failing test on `main`
**Steps:**
1. Push commit to `main`
2. CI test job fails
**Expected Result:** Publish job does not run; no image is pushed to registry
**Screenshot Checkpoint:** N/A
**Maps to:** TBD

## 4. Data Model

No new data entities. Version is a build-time constant embedded in the binary.

### Build-time Variables

| Variable | Type | Source | Description |
|----------|------|--------|-------------|
| `main.version` | string | Git tag or `beta-{SHA}` | Embedded via `-ldflags` |

## 5. API Contract

### GET /health (modified)

**Description:** Returns service health status — now includes build version.

**Response (200):**
```json
{
  "status": "ok",
  "mongo": "connected",
  "version": "v0.5.0"
}
```

No new endpoints. Only modification is adding `version` to existing `/health` response.

## 6. UI Behavior

N/A — no UI component.

## 7. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Registry unreachable during push | CI publish job fails, test job status unaffected |
| Tag pushed without passing tests | Publish job waits for test job; if tests fail, publish is skipped |
| Binary built without `-ldflags` (local dev) | `version` defaults to `"dev"` |
| Tag format is not `v*` (e.g., `release-1`) | Publish job does not trigger — only `v*` tags |
| Push to non-main branch | No publish job runs (beta or versioned) |

## 8. Dependencies

- GitHub Actions secrets: `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`
- Registry accessible from GitHub Actions runners: `dockerhub.calebdunn.tech`
- Existing Dockerfile (multi-stage alpine build)
- Existing CI workflow (`.github/workflows/ci.yml`)

## 9. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-07 | 0.1.0 | calebdunn | Initial spec from /add:spec interview |
