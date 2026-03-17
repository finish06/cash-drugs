# Staging Deployment Guide

Shared across: **cash-drugs**, **drug-gate**, **drugs-quiz**

## Staging Host

| Property | Value |
|----------|-------|
| Host | 192.168.1.145 |
| SSH | `ssh staging1` (key at `~/.ssh/staging1`) |
| Deploy path | `/opt/{service-name}/` |
| Docker network | `internal` (shared across all services) |
| Auto-update | Cron every 5 min (`/opt/staging-autopull.sh`) |

## Services on Staging

| Service | Port | Container | Image Tag | Path |
|---------|------|-----------|-----------|------|
| cash-drugs (nginx LB) | 8083 | `cash-drugs` | `nginx:alpine` | `/opt/cash-drugs/` |
| cash-drugs leader | 8085 | `cash-drugs-leader` | `:beta` | `/opt/cash-drugs/` |
| cash-drugs replica | 8086 | `cash-drugs-replica` | `:beta` | `/opt/cash-drugs/` |
| cash-drugs-mongo | (internal) | `cash-drugs-mongo` | `mongo:4.4` | `/opt/cash-drugs/` |
| drug-gate | 8082 | `drug-gate` | `:beta` | `/opt/drug-gate/` |
| drug-gate-redis | (internal) | `drug-gate-redis` | `redis:alpine` | `/opt/drug-gate/` |
| drugs-quiz | 8080 | `drugs-quiz` | `:beta-*` | `/opt/drugs-quiz/` |

### Multi-Instance Architecture (cash-drugs)

```
drug-gate (8082)
    ↓
cash-drugs (nginx LB, 8083) ──→ least_conn
    ├── cash-drugs-leader (8085)  ENABLE_SCHEDULER=true
    └── cash-drugs-replica (8086) ENABLE_SCHEDULER=false
    ↓
cash-drugs-mongo (MongoDB 4.4, shared)
```

- **Leader** runs scheduler (cron refresh) and startup warmup (196 parameterized queries)
- **Replica** serves requests only — no scheduler, `/ready` returns 200 immediately
- Both read/write same MongoDB database (`drugs_staging`)
- Nginx uses `least_conn` to distribute by active connections
- Container name `cash-drugs` preserves DNS for drug-gate (no config change needed)

### Direct Instance Access

```bash
# Via load balancer (normal)
curl http://192.168.1.145:8083/health

# Direct to leader (debugging, metrics)
curl http://192.168.1.145:8085/version  # leader=true
curl http://192.168.1.145:8085/metrics

# Direct to replica
curl http://192.168.1.145:8086/version  # leader=false
curl http://192.168.1.145:8086/metrics
```

### Prometheus Scraping

Scrape instances directly (not the LB):
```yaml
- targets:
    - staging1.du.nn:8085  # leader
    - staging1.du.nn:8086  # replica
  labels:
    app: cashdrugs
    env: staging
```

### Inter-service connectivity

All services are on the `internal` Docker network. Use container names for DNS:

```
drug-gate → http://cash-drugs:8080  (via nginx LB → leader/replica)
drug-gate → http://drug-gate-redis:6379  (Redis)
```

## How Auto-Update Works

A cron job runs `/opt/staging-autopull.sh` every 5 minutes:

```bash
*/5 * * * * /opt/staging-autopull.sh
```

The script loops through `/opt/cash-drugs`, `/opt/drug-gate`, `/opt/drugs-quiz`:
1. Finds the compose file (`compose.yaml` or `docker-compose.yml`)
2. Runs `docker compose pull -q` (silent pull)
3. Runs `docker compose up -d --remove-orphans` (restarts only if image changed)
4. Logs to syslog: `logger -t staging-autopull`

**This replaces Watchtower** — simpler, no version compatibility issues, works with the private registry.

### What triggers an update

Push to `main` → CI builds `:beta` image → within 5 minutes, staging pulls and restarts.

### Checking update logs

```bash
ssh staging1 "journalctl -t staging-autopull --since '1 hour ago'"
```

## How to Deploy / Update

### Automatic (default)

Push to `main`. CI builds `:beta`. Cron pulls within 5 min. Done.

### Manual (immediate)

```bash
ssh staging1 "/opt/staging-autopull.sh"
```

Or for a specific service:

```bash
ssh staging1 "cd /opt/cash-drugs && docker compose pull && docker compose up -d"
```

### Update config (no image change)

```bash
scp file staging1:/opt/cash-drugs/config.yaml
ssh staging1 "cd /opt/cash-drugs && docker compose restart drugs"
```

### Update compose file

```bash
scp docker-compose.staging.yml staging1:/opt/cash-drugs/compose.yaml
ssh staging1 "cd /opt/cash-drugs && docker compose up -d"
```

## SSH Access

All projects use the same SSH key:

```
~/.ssh/staging1        (private key)
~/.ssh/staging1.pub    (public key)
```

SSH config entry (`~/.ssh/config`):

```
Host staging1
    HostName 192.168.1.145
    User finish06
    IdentityFile ~/.ssh/staging1
    StrictHostKeyChecking no
```

Usage: `ssh staging1`, `scp file staging1:/path`

## Viewing Logs

```bash
# All containers
ssh staging1 "docker ps --format '{{.Names}}' | xargs -I{} docker logs --tail 5 {}"

# Specific service
ssh staging1 "docker logs --tail 50 -f cash-drugs"
ssh staging1 "docker logs --tail 50 -f drug-gate"
ssh staging1 "docker logs --tail 50 -f drugs-quiz"

# Via Dozzle (web UI)
# Dozzle agent is running on staging — check Pangolin for the URL
```

## Conventions

1. **Deploy path:** Always `/opt/{service-name}/`
2. **Compose file:** `compose.yaml` (preferred) or `docker-compose.yml`
3. **Network:** Always `internal` (external, shared)
4. **Container names:** Explicit (`container_name: {service}`) for DNS resolution
5. **Image tag:** `:beta` for staging, `:latest` for production
6. **No Watchtower** — cron-based auto-pull handles all services uniformly
7. **MongoDB:** Use `mongo:4.4` on this host (CPU lacks AVX for 5.0+)

## Troubleshooting

### Service won't start
```bash
ssh staging1 "cd /opt/{service} && docker compose logs --tail 20"
```

### Image not pulling
Check registry auth:
```bash
ssh staging1 "docker login dockerhub.calebdunn.tech"
```
Auth config is at `~/.docker/config.json` and `/opt/drug-gate/docker-config.json`.

### DNS resolution between containers
Both containers must be on the `internal` network:
```bash
ssh staging1 "docker network inspect internal --format '{{range .Containers}}{{.Name}} {{end}}'"
```

### MongoDB AVX error
Use `mongo:4.4`, not `mongo:latest`. The staging host CPU doesn't support AVX (required by MongoDB 5.0+).
