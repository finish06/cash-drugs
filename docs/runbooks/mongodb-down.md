# Runbook: MongoDB Down

**Alert:** `CashDrugsMongoDBDown`
**Severity:** critical
**SLO Impact:** Availability (99.5%), Stale-serve guarantee (100%)

## Symptom

The alert fires when `cashdrugs_mongodb_up == 0` for more than 1 minute. All cache lookups will miss (falling through to upstream), upstream fetch results cannot be stored, and the LRU in-memory cache becomes the only serving layer. Once the LRU evicts entries, consumers start seeing errors or degraded responses.

The `/health` endpoint will report MongoDB as unhealthy. Downstream consumers may see increased latency or missing data.

## Likely Cause

1. **MongoDB container crashed or was OOM-killed** -- most common in staging where the 2GB limit is tight
2. **MongoDB storage volume full** -- bind mount (`/mnt/mongo` in production) or Docker volume ran out of space
3. **Network partition** -- MongoDB container is running but the app container cannot reach it (Docker network issue)
4. **MongoDB is in a slow startup or repair cycle** -- WiredTiger journal recovery after unclean shutdown
5. **Connection string misconfigured** -- `MONGO_URI` environment variable changed or malformed after a redeploy

## Diagnostic Steps

### 1. Check service health

```bash
# Staging
curl -s http://192.168.1.145:8083/health | jq .
curl -s http://192.168.1.145:8085/health | jq .   # leader direct
curl -s http://192.168.1.145:8086/health | jq .   # replica direct

# Production
curl -s http://192.168.1.86:8083/health | jq .
```

### 2. Check MongoDB container status

```bash
# Staging
ssh 192.168.1.145
docker ps -a --filter name=cash-drugs-mongo
docker logs cash-drugs-mongo --tail 100 --since 5m

# Production
ssh 192.168.1.86
docker ps -a --filter name=mongodb
docker logs cash-drugs-mongodb-1 --tail 100 --since 5m
```

### 3. Check if MongoDB is accepting connections

```bash
# From the host where MongoDB runs
docker exec cash-drugs-mongo mongosh --eval "db.adminCommand('ping')"
```

### 4. Check disk space

```bash
# Staging (Docker volume)
docker system df -v | grep mongo-staging-data
df -h

# Production (bind mount)
df -h /mnt/mongo
du -sh /mnt/mongo
```

### 5. Check for OOM kills

```bash
dmesg | grep -i "oom\|killed" | tail 20
docker inspect cash-drugs-mongo --format '{{.State.OOMKilled}}'
```

### 6. Check metrics

```
# PromQL: confirm MongoDB is reported down
cashdrugs_mongodb_up

# PromQL: check when it went down
changes(cashdrugs_mongodb_up[1h])
```

## Remediation

### Quick Fix

**Restart the MongoDB container:**

```bash
# Staging
cd /path/to/cash-drugs && docker compose -f docker-compose.staging.yml restart mongodb

# Production
cd /path/to/cash-drugs && docker compose -f docker-compose.prod.yml restart mongodb
```

Wait 30 seconds, then verify:

```bash
curl -s http://192.168.1.145:8083/health | jq .
```

The app instances will automatically reconnect. No app restart is needed -- the MongoDB client retries connections.

**If MongoDB was OOM-killed (staging):**

```bash
# Check current WiredTiger cache allocation
docker logs cash-drugs-mongo 2>&1 | grep -i "wiredtiger"

# The staging config limits WiredTiger to 1GB within a 2GB container limit.
# If data has grown, consider increasing the container limit in docker-compose.staging.yml.
```

### Root Cause Fix

**Disk full:**

```bash
# Clean old MongoDB data or compact
docker exec cash-drugs-mongo mongosh --eval "db.runCommand({compact: 'cache_entries'})"

# Or expand the volume/mount
```

**OOM-killed repeatedly:**

Edit the relevant docker-compose file to increase the memory limit:

```yaml
# docker-compose.staging.yml
mongodb:
  deploy:
    resources:
      limits:
        memory: 3g
  command: ["mongod", "--wiredTigerCacheSizeGB", "1.5"]
```

Then redeploy:

```bash
docker compose -f docker-compose.staging.yml up -d mongodb
```

**Network partition:**

```bash
# Verify both containers are on the same Docker network
docker network inspect internal
# Look for both MongoDB and app containers in the "Containers" section
```

## Escalation

- **P1 incident** -- acknowledge within 15 minutes, investigate within 30 minutes, resolve within 2 hours
- If MongoDB cannot be restarted after two attempts, check host-level issues (disk, memory, Docker daemon)
- If the host itself is unreachable, this is an infrastructure incident -- escalate to hardware/network investigation
- If data corruption is suspected (MongoDB won't start, journal errors), do NOT attempt repair without a backup

## Prevention

- Monitor disk usage on MongoDB volumes with a separate alert (e.g., > 80% full)
- Set `wiredTigerCacheSizeGB` explicitly to prevent MongoDB from consuming all available memory
- Ensure `restart: unless-stopped` is set on the MongoDB container (already configured)
- Consider periodic `mongodump` backups so recovery from corruption is possible
- Review memory limits quarterly as data volume grows
