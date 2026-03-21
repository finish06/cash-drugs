# Runbook: Scheduler Stalled

**Alert:** `CashDrugsSchedulerStalled`
**Severity:** warning
**SLO Impact:** Upstream fetch success rate (> 95% -- stale data is served but freshness degrades), Stale-serve guarantee (indirectly -- if data expires without refresh, stale-serve coverage shrinks)

## Symptom

The alert fires when the leader instance (`cashdrugs_instance_leader == 1`) has not recorded any scheduler runs in the last hour (`increase(cashdrugs_scheduler_runs_total[1h]) == 0`). This means cached data is not being refreshed on schedule. Existing cached data will continue to be served, but it will grow stale. Once TTLs expire, consumers may receive outdated information.

The scheduler only runs on the leader instance (`ENABLE_SCHEDULER=true`). Replicas do not run the scheduler.

## Likely Cause

1. **Leader container is down or crash-looping** -- the scheduler cannot run if the leader is not running
2. **`ENABLE_SCHEDULER` set to `false` on the leader** -- misconfiguration after a redeploy
3. **Scheduler goroutine panicked or deadlocked** -- the cron loop exited silently
4. **No endpoints configured with `refresh` schedules** -- `config.yaml` has no cron-scheduled slugs
5. **All scheduled runs completed before the 1-hour window** -- the cron schedules are longer than 1 hour and the window was quiet (this is a false positive)
6. **Leader is running but not recognized as leader** -- `cashdrugs_instance_leader` metric not being set

## Diagnostic Steps

### 1. Check if the leader is running

```bash
# Staging
docker ps --filter name=cash-drugs-leader
curl -s http://192.168.1.145:8085/health | jq .

# Production
docker ps --filter name=cash-drugs-drugs
curl -s http://192.168.1.86:8083/health | jq .
```

### 2. Check scheduler configuration

```bash
# Verify ENABLE_SCHEDULER is true on the leader
docker inspect cash-drugs-leader --format '{{range .Config.Env}}{{println .}}{{end}}' | grep SCHEDULER
```

### 3. Check scheduler logs

```bash
# Staging
docker logs cash-drugs-leader --tail 200 --since 2h 2>&1 | grep -i "scheduler\|cron\|refresh\|schedule"

# Production
docker logs cash-drugs-drugs-1 --tail 200 --since 2h 2>&1 | grep -i "scheduler\|cron\|refresh\|schedule"
```

Look for:
- "scheduler started" at startup
- "scheduler run" entries showing cron executions
- Panic or error messages indicating the scheduler goroutine crashed

### 4. Check the metrics

```
# PromQL: scheduler run count
cashdrugs_scheduler_runs_total

# PromQL: is the leader metric set?
cashdrugs_instance_leader

# PromQL: last increase (should be > 0 if scheduler is running)
increase(cashdrugs_scheduler_runs_total[2h])
```

### 5. Check configured schedules

```bash
# Review which slugs have refresh schedules
grep -A1 "refresh:" /path/to/config.yaml
```

Current refresh schedules from `config.yaml`:
- `drugnames`: `0 */6 * * *` (every 6 hours)
- `spls`: `0 */6 * * *` (every 6 hours)
- `drugclasses`: `0 */6 * * *` (every 6 hours)
- `fda-enforcement`: `0 3 * * *` (daily at 3 AM)
- `fda-shortages`: `0 3 * * *` (daily at 3 AM)

If the alert fires between scheduled runs (e.g., 2 AM when the next run is 3 AM), it may be a false positive.

### 6. Verify it is not a false positive

The alert requires 1 hour with zero runs. If the most frequent schedule is every 6 hours, the scheduler may legitimately have no runs in a given hour. Check if the last run was within the expected cron interval:

```
# PromQL: when was the last scheduler run?
time() - cashdrugs_scheduler_last_run_timestamp
```

## Remediation

### Quick Fix

**Restart the leader container:**

```bash
# Staging
docker compose -f docker-compose.staging.yml restart drugs-leader

# Production
docker compose -f docker-compose.prod.yml restart drugs
```

After restart, check logs to confirm the scheduler starts:

```bash
docker logs cash-drugs-leader --tail 20 2>&1 | grep -i "scheduler"
```

**If `ENABLE_SCHEDULER` was accidentally set to `false`:**

```bash
# Edit docker-compose.staging.yml: set ENABLE_SCHEDULER=true on the leader
docker compose -f docker-compose.staging.yml up -d drugs-leader
```

**Manually trigger a refresh (if supported):**

```bash
# Force-refresh a specific slug (subject to 30-second cooldown)
curl -X POST http://192.168.1.145:8085/api/v1/drugnames?force=true
```

### Root Cause Fix

**Scheduler goroutine panicked:**

This is a code bug. Check logs for panic stack traces:

```bash
docker logs cash-drugs-leader --tail 500 2>&1 | grep -A 20 "panic\|runtime error"
```

File a bug with the stack trace. As a workaround, set up a periodic container restart via cron on the host:

```bash
# Temporary workaround: restart leader daily at 2 AM
echo "0 2 * * * docker restart cash-drugs-leader" | crontab -
```

**False positive due to long cron intervals:**

If the most frequent schedule is every 6 hours, the 1-hour alert window will produce false positives. Consider adjusting the alert to use a longer window:

```yaml
# alerts.yml: increase window to match longest expected gap
expr: >-
  sum by (instance) (
    increase(cashdrugs_scheduler_runs_total[7h])
  ) == 0
  and cashdrugs_instance_leader == 1
for: 1h
```

## Escalation

- **P3** if the scheduler is stalled but cached data is still within TTL -- data is stale but being served
- **P2** if cached data has expired for some slugs and consumers are getting empty or very old results
- **P1** if the leader instance is down entirely -- combine with other alerts (MongoDB down, etc.)
- Check the replica instance to confirm it is still serving traffic while the leader is being investigated

## Prevention

- Add a scheduler heartbeat log entry (even when no cron jobs are due) to distinguish "no jobs scheduled" from "scheduler crashed"
- Monitor `cashdrugs_scheduler_runs_total` rate as a leading indicator
- Ensure the alert window is longer than the longest gap between scheduled runs to avoid false positives
- Set generous TTLs so data survives scheduler interruptions
- Consider adding a `/health` check that includes scheduler liveness (not just MongoDB connectivity)
