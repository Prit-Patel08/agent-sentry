#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DAYS=7
API_BASE="${API_BASE:-http://127.0.0.1:8080}"
DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}"
OUT_DIR=""
FAIL_ON_BREACH=0
REPLAY_MAX_ROWS=50000
REPLAY_SPIKE_YELLOW=5
REPLAY_SPIKE_RED=10
CONFLICT_SPIKE_YELLOW=2
CONFLICT_SPIKE_RED=5
SPIKE_FACTOR_YELLOW="2.00"
SPIKE_FACTOR_RED="3.00"

usage() {
  cat <<EOF
Usage: ./scripts/slo_weekly_review.sh [options]

Options:
  --days N             Window size in days (default: 7).
  --api-base URL       FlowForge API base URL (default: http://127.0.0.1:8080).
  --db PATH            SQLite DB path (default: FLOWFORGE_DB_PATH or flowforge.db).
  --out DIR            Output directory (default: pilot_artifacts/slo-weekly-<timestamp>).
  --replay-max-rows N  Replay ledger row-cap target for SLO (default: 50000, <=0 disables check).
  --replay-spike-yellow N
                      Yellow threshold for daily replay events (default: 5).
  --replay-spike-red N
                      Red threshold for daily replay events (default: 10).
  --conflict-spike-yellow N
                      Yellow threshold for daily conflict events (default: 2).
  --conflict-spike-red N
                      Red threshold for daily conflict events (default: 5).
  --fail-on-breach     Exit non-zero if error budget status is RED.
  -h, --help           Show help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days)
      DAYS="${2:-}"
      shift 2
      ;;
    --api-base)
      API_BASE="${2:-}"
      shift 2
      ;;
    --db)
      DB_PATH="${2:-}"
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --replay-max-rows)
      REPLAY_MAX_ROWS="${2:-}"
      shift 2
      ;;
    --replay-spike-yellow)
      REPLAY_SPIKE_YELLOW="${2:-}"
      shift 2
      ;;
    --replay-spike-red)
      REPLAY_SPIKE_RED="${2:-}"
      shift 2
      ;;
    --conflict-spike-yellow)
      CONFLICT_SPIKE_YELLOW="${2:-}"
      shift 2
      ;;
    --conflict-spike-red)
      CONFLICT_SPIKE_RED="${2:-}"
      shift 2
      ;;
    --fail-on-breach)
      FAIL_ON_BREACH=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ ! "$DAYS" =~ ^[0-9]+$ ]] || [[ "$DAYS" -le 0 ]]; then
  echo "--days must be a positive integer (got: $DAYS)" >&2
  exit 1
fi
if [[ ! "$REPLAY_MAX_ROWS" =~ ^-?[0-9]+$ ]]; then
  echo "--replay-max-rows must be an integer (got: $REPLAY_MAX_ROWS)" >&2
  exit 1
fi
for numeric in "$REPLAY_SPIKE_YELLOW" "$REPLAY_SPIKE_RED" "$CONFLICT_SPIKE_YELLOW" "$CONFLICT_SPIKE_RED"; do
  if [[ ! "$numeric" =~ ^[0-9]+$ ]]; then
    echo "Replay spike thresholds must be non-negative integers." >&2
    exit 1
  fi
done
if [[ "$REPLAY_SPIKE_YELLOW" -le 0 ]] || [[ "$REPLAY_SPIKE_RED" -le 0 ]] || [[ "$CONFLICT_SPIKE_YELLOW" -le 0 ]] || [[ "$CONFLICT_SPIKE_RED" -le 0 ]]; then
  echo "Replay spike thresholds must be positive integers." >&2
  exit 1
fi
if [[ "$REPLAY_SPIKE_YELLOW" -gt "$REPLAY_SPIKE_RED" ]]; then
  echo "--replay-spike-yellow must be <= --replay-spike-red." >&2
  exit 1
fi
if [[ "$CONFLICT_SPIKE_YELLOW" -gt "$CONFLICT_SPIKE_RED" ]]; then
  echo "--conflict-spike-yellow must be <= --conflict-spike-red." >&2
  exit 1
fi
REPLAY_MAX_ROWS_TARGET_LABEL="$REPLAY_MAX_ROWS"
if [[ "$REPLAY_MAX_ROWS" -le 0 ]]; then
  REPLAY_MAX_ROWS_TARGET_LABEL="DISABLED"
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/slo-weekly-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"

for cmd in sqlite3 curl awk; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
done

if [[ ! -f "$DB_PATH" ]]; then
  echo "Database file not found: $DB_PATH" >&2
  echo "Set FLOWFORGE_DB_PATH or pass --db PATH." >&2
  exit 1
fi

WINDOW="-${DAYS} day"
RUN_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

sql_scalar() {
  local query="$1"
  sqlite3 -batch -noheader "$DB_PATH" "$query"
}

EVENT_TIME_COL="created_at"
if [[ "$(sql_scalar "SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='created_at';")" -eq 0 ]]; then
  EVENT_TIME_COL="timestamp"
fi

num_le() {
  local left="$1"
  local right="$2"
  awk -v a="$left" -v b="$right" 'BEGIN { exit !(a <= b) }'
}

num_ge() {
  local left="$1"
  local right="$2"
  awk -v a="$left" -v b="$right" 'BEGIN { exit !(a >= b) }'
}

promote_replay_spike_status() {
  local trigger="$1"
  local reason="$2"
  replay_spike_reasons+=("$reason")

  if [[ "$trigger" == "RED" ]]; then
    slo_c_replay_spike_status="FAIL"
    replay_spike_budget_trigger="RED"
    return
  fi

  if [[ "$replay_spike_budget_trigger" != "RED" ]]; then
    replay_spike_budget_trigger="YELLOW"
  fi
  if [[ "$slo_c_replay_spike_status" == "PASS" ]]; then
    slo_c_replay_spike_status="WARN"
  fi
}

probe_failures=0

probe() {
  local endpoint="$1"
  local outfile="$2"
  if curl -fsS --max-time 4 "${API_BASE}${endpoint}" >"$outfile" 2>"$outfile.err"; then
    rm -f "$outfile.err"
    return 0
  fi
  probe_failures=$((probe_failures + 1))
  {
    echo "{\"error\":\"probe failed\",\"endpoint\":\"${endpoint}\"}"
    cat "$outfile.err" 2>/dev/null || true
  } >"$outfile"
  rm -f "$outfile.err"
  return 1
}

probe "/healthz" "$OUT_DIR/healthz.json" || true
probe "/readyz" "$OUT_DIR/readyz.json" || true
probe "/metrics" "$OUT_DIR/metrics.prom" || true
probe "/timeline" "$OUT_DIR/timeline.json" || true
probe "/v1/ops/controlplane/replay/history?days=${DAYS}" "$OUT_DIR/replay_history.json" || true

TOTAL_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}');")"
INCIDENT_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='incident';")"
DECISION_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='decision';")"
AUDIT_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='audit';")"
POLICY_DRY_RUN_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='policy_dry_run';")"
DISTINCT_INCIDENTS="$(sql_scalar "SELECT COUNT(DISTINCT incident_id) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND incident_id IS NOT NULL AND incident_id != '';")"
REPLAY_TABLE_EXISTS="$(sql_scalar "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='control_plane_replays';")"

REPLAY_ROW_COUNT=0
REPLAY_OLDEST_AGE_SECONDS=0
if [[ "$REPLAY_TABLE_EXISTS" -eq 1 ]]; then
  REPLAY_ROW_COUNT="$(sql_scalar "SELECT COUNT(*) FROM control_plane_replays;")"
  REPLAY_OLDEST_AGE_SECONDS="$(sql_scalar "SELECT COALESCE(CAST((julianday('now') - julianday(MIN(last_seen_at))) * 86400 AS INTEGER), 0) FROM control_plane_replays;")"
fi

IDEMPOTENT_REPLAY_EVENTS="$(sql_scalar "
SELECT COUNT(*) FROM events
WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND event_type='audit'
  AND UPPER(title)='IDEMPOTENT_REPLAY';
")"

IDEMPOTENT_CONFLICT_EVENTS="$(sql_scalar "
SELECT COUNT(*) FROM events
WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND event_type='audit'
  AND UPPER(title)='IDEMPOTENT_CONFLICT';
")"

REPLAY_TREND_TSV="$OUT_DIR/replay_daily_trend.tsv"
{
  printf 'day\treplay_events\tconflict_events\n'
  sqlite3 -batch -noheader -separator $'\t' "$DB_PATH" "
WITH RECURSIVE seq(n) AS (
  SELECT 0
  UNION ALL
  SELECT n + 1
  FROM seq
  WHERE n + 1 < ${DAYS}
),
daily AS (
  SELECT
    date(COALESCE(${EVENT_TIME_COL}, timestamp, CURRENT_TIMESTAMP)) AS day,
    SUM(CASE WHEN event_type='audit' AND UPPER(title)='IDEMPOTENT_REPLAY' THEN 1 ELSE 0 END) AS replay_events,
    SUM(CASE WHEN event_type='audit' AND UPPER(title)='IDEMPOTENT_CONFLICT' THEN 1 ELSE 0 END) AS conflict_events
  FROM events
  WHERE date(COALESCE(${EVENT_TIME_COL}, timestamp, CURRENT_TIMESTAMP)) >= date('now', '-$((DAYS - 1)) day')
  GROUP BY day
),
series AS (
  SELECT date('now', printf('-%d day', ${DAYS} - 1 - n)) AS day
  FROM seq
)
SELECT
  s.day,
  COALESCE(d.replay_events, 0),
  COALESCE(d.conflict_events, 0)
FROM series s
LEFT JOIN daily d USING(day)
ORDER BY s.day ASC;
"
} >"$REPLAY_TREND_TSV"

LATEST_REPLAY_DAY="$(awk -F '\t' 'NR > 1 { day = $1 } END { print day }' "$REPLAY_TREND_TSV")"
LATEST_REPLAY_EVENTS="$(awk -F '\t' 'NR > 1 { value = $2 } END { print value + 0 }' "$REPLAY_TREND_TSV")"
LATEST_CONFLICT_EVENTS="$(awk -F '\t' 'NR > 1 { value = $3 } END { print value + 0 }' "$REPLAY_TREND_TSV")"
PREV_REPLAY_PEAK="$(awk -F '\t' '
NR > 1 { count += 1; replay[count] = $2 + 0 }
END {
  peak = 0
  for (i = 1; i < count; i++) {
    if (replay[i] > peak) {
      peak = replay[i]
    }
  }
  print peak + 0
}
' "$REPLAY_TREND_TSV")"
PREV_CONFLICT_PEAK="$(awk -F '\t' '
NR > 1 { count += 1; conflict[count] = $3 + 0 }
END {
  peak = 0
  for (i = 1; i < count; i++) {
    if (conflict[i] > peak) {
      peak = conflict[i]
    }
  }
  print peak + 0
}
' "$REPLAY_TREND_TSV")"

REPLAY_RATIO_TO_PREV_PEAK="N/A"
if [[ "$PREV_REPLAY_PEAK" -gt 0 ]]; then
  REPLAY_RATIO_TO_PREV_PEAK="$(awk -v latest="$LATEST_REPLAY_EVENTS" -v prev="$PREV_REPLAY_PEAK" 'BEGIN { printf "%.2f", latest / prev }')"
fi
CONFLICT_RATIO_TO_PREV_PEAK="N/A"
if [[ "$PREV_CONFLICT_PEAK" -gt 0 ]]; then
  CONFLICT_RATIO_TO_PREV_PEAK="$(awk -v latest="$LATEST_CONFLICT_EVENTS" -v prev="$PREV_CONFLICT_PEAK" 'BEGIN { printf "%.2f", latest / prev }')"
fi

DESTRUCTIVE_ACTIONS="$(sql_scalar "
SELECT COUNT(*) FROM events
WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND event_type='audit'
  AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE');
")"

ACTION_INCIDENTS="$(sql_scalar "
WITH action_incidents AS (
  SELECT DISTINCT incident_id
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND incident_id IS NOT NULL
    AND incident_id != ''
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE')
)
SELECT COUNT(*) FROM action_incidents;
")"

LOW_CONFIDENCE_ACTION_INCIDENTS="$(sql_scalar "
WITH action_incidents AS (
  SELECT DISTINCT incident_id
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND incident_id IS NOT NULL
    AND incident_id != ''
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE')
)
SELECT COUNT(*)
FROM events e
WHERE e.${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND e.event_type='incident'
  AND e.incident_id IN (SELECT incident_id FROM action_incidents)
  AND e.confidence_score < 0.60;
")"

LOW_CONFIDENCE_RATIO="N/A"
if [[ "$ACTION_INCIDENTS" -gt 0 ]]; then
  LOW_CONFIDENCE_RATIO="$(awk -v low="$LOW_CONFIDENCE_ACTION_INCIDENTS" -v total="$ACTION_INCIDENTS" 'BEGIN { printf "%.4f", low/total }')"
fi

LATENCY_SAMPLE_COUNT="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
)
SELECT COUNT(*) FROM latencies;
")"

LATENCY_P95="N/A"
LATENCY_AVG="N/A"
if [[ "$LATENCY_SAMPLE_COUNT" -gt 0 ]]; then
  LATENCY_P95="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
),
ranked AS (
  SELECT latency_s,
         ROW_NUMBER() OVER (ORDER BY latency_s) AS rn,
         COUNT(*) OVER () AS n
  FROM latencies
)
SELECT printf('%.2f', latency_s)
FROM ranked
WHERE rn = ((n * 95 + 99) / 100)
LIMIT 1;
")"

  LATENCY_AVG="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
)
SELECT printf('%.2f', AVG(latency_s)) FROM latencies;
")"
fi

RESTART_STORM_VIOLATIONS="$(sql_scalar "
WITH restarts AS (
  SELECT
    run_id,
    strftime('%Y-%m-%d %H:', ${EVENT_TIME_COL}) || printf('%02d', (CAST(strftime('%M', ${EVENT_TIME_COL}) AS INTEGER) / 10) * 10) AS bucket_10m,
    COUNT(*) AS restart_count
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND UPPER(title) = 'AUTO_RESTART'
  GROUP BY run_id, bucket_10m
)
SELECT COUNT(*) FROM restarts WHERE restart_count > 3;
")"

LATEST_EVENT_AGE_SECONDS="$(sql_scalar "
SELECT COALESCE(CAST((julianday('now') - julianday(MAX(${EVENT_TIME_COL}))) * 86400 AS INTEGER), -1)
FROM events;
")"

UPTIME_SECONDS="N/A"
KILL_TOTAL="N/A"
RESTART_TOTAL="N/A"
IDEMPOTENT_REPLAY_TOTAL="N/A"
IDEMPOTENT_CONFLICT_TOTAL="N/A"
REPLAY_ROWS_GAUGE="N/A"
REPLAY_OLDEST_AGE_GAUGE="N/A"
if [[ -f "$OUT_DIR/metrics.prom" ]]; then
  UPTIME_SECONDS="$(awk '/^flowforge_uptime_seconds /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  KILL_TOTAL="$(awk '/^flowforge_process_kill_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  RESTART_TOTAL="$(awk '/^flowforge_process_restart_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  IDEMPOTENT_REPLAY_TOTAL="$(awk '/^flowforge_controlplane_idempotent_replay_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  IDEMPOTENT_CONFLICT_TOTAL="$(awk '/^flowforge_controlplane_idempotency_conflict_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  REPLAY_ROWS_GAUGE="$(awk '/^flowforge_controlplane_replay_rows /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  REPLAY_OLDEST_AGE_GAUGE="$(awk '/^flowforge_controlplane_replay_oldest_age_seconds /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  UPTIME_SECONDS="${UPTIME_SECONDS:-N/A}"
  KILL_TOTAL="${KILL_TOTAL:-N/A}"
  RESTART_TOTAL="${RESTART_TOTAL:-N/A}"
  IDEMPOTENT_REPLAY_TOTAL="${IDEMPOTENT_REPLAY_TOTAL:-N/A}"
  IDEMPOTENT_CONFLICT_TOTAL="${IDEMPOTENT_CONFLICT_TOTAL:-N/A}"
  REPLAY_ROWS_GAUGE="${REPLAY_ROWS_GAUGE:-N/A}"
  REPLAY_OLDEST_AGE_GAUGE="${REPLAY_OLDEST_AGE_GAUGE:-N/A}"
fi

slo_a_latency_status="NO_DATA"
slo_a_precision_status="NO_DATA"
slo_b_storm_status="PASS"
slo_c_api_status="FAIL"
slo_c_freshness_status="NO_DATA"
slo_c_idempotency_status="PASS"
slo_c_replay_capacity_status="NO_DATA"
slo_c_replay_spike_status="PASS"
replay_spike_budget_trigger="NONE"
replay_spike_reasons=()

if [[ "$LATENCY_SAMPLE_COUNT" -gt 0 ]]; then
  if num_le "$LATENCY_P95" "15"; then
    slo_a_latency_status="PASS"
  else
    slo_a_latency_status="FAIL"
  fi
fi

if [[ "$ACTION_INCIDENTS" -gt 0 ]]; then
  if num_le "$LOW_CONFIDENCE_RATIO" "0.10"; then
    slo_a_precision_status="PASS"
  else
    slo_a_precision_status="FAIL"
  fi
fi

if [[ "$RESTART_STORM_VIOLATIONS" -gt 0 ]]; then
  slo_b_storm_status="FAIL"
fi

if [[ "$probe_failures" -eq 0 ]]; then
  slo_c_api_status="PASS"
fi

if [[ "$LATEST_EVENT_AGE_SECONDS" -ge 0 ]]; then
  if num_le "$LATEST_EVENT_AGE_SECONDS" "300"; then
    slo_c_freshness_status="PASS"
  else
    slo_c_freshness_status="FAIL"
  fi
fi

if [[ "$IDEMPOTENT_CONFLICT_EVENTS" -gt 0 ]]; then
  slo_c_idempotency_status="FAIL"
fi

if [[ "$REPLAY_MAX_ROWS" -gt 0 ]]; then
  if num_le "$REPLAY_ROW_COUNT" "$REPLAY_MAX_ROWS"; then
    slo_c_replay_capacity_status="PASS"
  else
    slo_c_replay_capacity_status="FAIL"
  fi
fi

if [[ "$LATEST_REPLAY_EVENTS" -ge "$REPLAY_SPIKE_RED" ]]; then
  promote_replay_spike_status \
    "RED" \
    "replay daily count ${LATEST_REPLAY_EVENTS} reached red threshold ${REPLAY_SPIKE_RED}"
elif [[ "$LATEST_REPLAY_EVENTS" -ge "$REPLAY_SPIKE_YELLOW" ]]; then
  promote_replay_spike_status \
    "YELLOW" \
    "replay daily count ${LATEST_REPLAY_EVENTS} reached yellow threshold ${REPLAY_SPIKE_YELLOW}"
fi

if [[ "$LATEST_CONFLICT_EVENTS" -ge "$CONFLICT_SPIKE_RED" ]]; then
  promote_replay_spike_status \
    "RED" \
    "conflict daily count ${LATEST_CONFLICT_EVENTS} reached red threshold ${CONFLICT_SPIKE_RED}"
elif [[ "$LATEST_CONFLICT_EVENTS" -ge "$CONFLICT_SPIKE_YELLOW" ]]; then
  promote_replay_spike_status \
    "YELLOW" \
    "conflict daily count ${LATEST_CONFLICT_EVENTS} reached yellow threshold ${CONFLICT_SPIKE_YELLOW}"
fi

if [[ "$PREV_REPLAY_PEAK" -gt 0 ]]; then
  replay_rel_yellow="$(awk -v peak="$PREV_REPLAY_PEAK" -v factor="$SPIKE_FACTOR_YELLOW" 'BEGIN { printf "%.4f", peak * factor }')"
  replay_rel_red="$(awk -v peak="$PREV_REPLAY_PEAK" -v factor="$SPIKE_FACTOR_RED" 'BEGIN { printf "%.4f", peak * factor }')"
  if num_ge "$LATEST_REPLAY_EVENTS" "$replay_rel_red"; then
    promote_replay_spike_status \
      "RED" \
      "replay daily count ${LATEST_REPLAY_EVENTS} is >= ${SPIKE_FACTOR_RED}x previous peak ${PREV_REPLAY_PEAK}"
  elif num_ge "$LATEST_REPLAY_EVENTS" "$replay_rel_yellow"; then
    promote_replay_spike_status \
      "YELLOW" \
      "replay daily count ${LATEST_REPLAY_EVENTS} is >= ${SPIKE_FACTOR_YELLOW}x previous peak ${PREV_REPLAY_PEAK}"
  fi
fi

if [[ "$PREV_CONFLICT_PEAK" -gt 0 ]]; then
  conflict_rel_yellow="$(awk -v peak="$PREV_CONFLICT_PEAK" -v factor="$SPIKE_FACTOR_YELLOW" 'BEGIN { printf "%.4f", peak * factor }')"
  conflict_rel_red="$(awk -v peak="$PREV_CONFLICT_PEAK" -v factor="$SPIKE_FACTOR_RED" 'BEGIN { printf "%.4f", peak * factor }')"
  if num_ge "$LATEST_CONFLICT_EVENTS" "$conflict_rel_red"; then
    promote_replay_spike_status \
      "RED" \
      "conflict daily count ${LATEST_CONFLICT_EVENTS} is >= ${SPIKE_FACTOR_RED}x previous peak ${PREV_CONFLICT_PEAK}"
  elif num_ge "$LATEST_CONFLICT_EVENTS" "$conflict_rel_yellow"; then
    promote_replay_spike_status \
      "YELLOW" \
      "conflict daily count ${LATEST_CONFLICT_EVENTS} is >= ${SPIKE_FACTOR_YELLOW}x previous peak ${PREV_CONFLICT_PEAK}"
  fi
fi

REPLAY_SPIKE_REASON="no replay/conflict spike detected"
if (( ${#replay_spike_reasons[@]} > 0 )); then
  REPLAY_SPIKE_REASON="$(IFS='; '; echo "${replay_spike_reasons[*]}")"
fi

fail_count=0
warn_count=0
for st in \
  "$slo_a_latency_status" \
  "$slo_a_precision_status" \
  "$slo_b_storm_status" \
  "$slo_c_api_status" \
  "$slo_c_freshness_status" \
  "$slo_c_idempotency_status" \
  "$slo_c_replay_capacity_status" \
  "$slo_c_replay_spike_status"; do
  if [[ "$st" == "FAIL" ]]; then
    fail_count=$((fail_count + 1))
  elif [[ "$st" == "WARN" ]]; then
    warn_count=$((warn_count + 1))
  fi
done

ERROR_BUDGET_STATUS="GREEN"
if [[ "$replay_spike_budget_trigger" == "RED" ]]; then
  ERROR_BUDGET_STATUS="RED"
elif [[ "$fail_count" -ge 2 ]]; then
  ERROR_BUDGET_STATUS="RED"
elif [[ "$fail_count" -eq 1 ]] || [[ "$warn_count" -gt 0 ]] || [[ "$replay_spike_budget_trigger" == "YELLOW" ]]; then
  ERROR_BUDGET_STATUS="YELLOW"
fi

cat >"$OUT_DIR/summary.tsv" <<EOF
run_timestamp	${RUN_TS}
window_days	${DAYS}
db_path	${DB_PATH}
api_base	${API_BASE}
replay_max_rows_target	${REPLAY_MAX_ROWS}
replay_spike_yellow_threshold	${REPLAY_SPIKE_YELLOW}
replay_spike_red_threshold	${REPLAY_SPIKE_RED}
conflict_spike_yellow_threshold	${CONFLICT_SPIKE_YELLOW}
conflict_spike_red_threshold	${CONFLICT_SPIKE_RED}
replay_spike_factor_yellow	${SPIKE_FACTOR_YELLOW}
replay_spike_factor_red	${SPIKE_FACTOR_RED}
total_events	${TOTAL_EVENTS}
incident_events	${INCIDENT_EVENTS}
decision_events	${DECISION_EVENTS}
audit_events	${AUDIT_EVENTS}
policy_dry_run_events	${POLICY_DRY_RUN_EVENTS}
distinct_incidents	${DISTINCT_INCIDENTS}
replay_table_exists	${REPLAY_TABLE_EXISTS}
replay_row_count	${REPLAY_ROW_COUNT}
replay_oldest_age_seconds	${REPLAY_OLDEST_AGE_SECONDS}
idempotent_replay_events	${IDEMPOTENT_REPLAY_EVENTS}
idempotent_conflict_events	${IDEMPOTENT_CONFLICT_EVENTS}
destructive_actions	${DESTRUCTIVE_ACTIONS}
action_incidents	${ACTION_INCIDENTS}
low_confidence_action_incidents	${LOW_CONFIDENCE_ACTION_INCIDENTS}
low_confidence_ratio	${LOW_CONFIDENCE_RATIO}
latency_sample_count	${LATENCY_SAMPLE_COUNT}
latency_p95_seconds	${LATENCY_P95}
latency_avg_seconds	${LATENCY_AVG}
restart_storm_violations	${RESTART_STORM_VIOLATIONS}
latest_event_age_seconds	${LATEST_EVENT_AGE_SECONDS}
probe_failures	${probe_failures}
latest_replay_day	${LATEST_REPLAY_DAY}
latest_replay_events	${LATEST_REPLAY_EVENTS}
latest_conflict_events	${LATEST_CONFLICT_EVENTS}
previous_replay_peak	${PREV_REPLAY_PEAK}
previous_conflict_peak	${PREV_CONFLICT_PEAK}
replay_ratio_to_previous_peak	${REPLAY_RATIO_TO_PREV_PEAK}
conflict_ratio_to_previous_peak	${CONFLICT_RATIO_TO_PREV_PEAK}
replay_spike_budget_trigger	${replay_spike_budget_trigger}
replay_spike_reason	${REPLAY_SPIKE_REASON}
controlplane_idempotent_replay_total	${IDEMPOTENT_REPLAY_TOTAL}
controlplane_idempotency_conflict_total	${IDEMPOTENT_CONFLICT_TOTAL}
controlplane_replay_rows_gauge	${REPLAY_ROWS_GAUGE}
controlplane_replay_oldest_age_seconds_gauge	${REPLAY_OLDEST_AGE_GAUGE}
slo_c_idempotency_status	${slo_c_idempotency_status}
slo_c_replay_capacity_status	${slo_c_replay_capacity_status}
slo_c_replay_spike_status	${slo_c_replay_spike_status}
warn_count	${warn_count}
fail_count	${fail_count}
error_budget_status	${ERROR_BUDGET_STATUS}
EOF

REPLAY_TREND_MARKDOWN_TABLE="$(awk -F '\t' '
BEGIN {
  print "| Day | Replays | Conflicts |"
  print "|---|---:|---:|"
}
NR > 1 {
  printf "| %s | %s | %s |\n", $1, $2, $3
}
' "$REPLAY_TREND_TSV")"

cat >"$OUT_DIR/slo_weekly_report.md" <<EOF
# FlowForge Weekly SLO Review

- Generated: \`${RUN_TS}\`
- Window: last **${DAYS} day(s)**
- Database: \`${DB_PATH}\`
- API base: \`${API_BASE}\`

## SLO Scoreboard

| SLO Objective | Target | Observed | Status |
|---|---:|---:|---|
| Detection->Action latency p95 | <= 15s | ${LATENCY_P95} | ${slo_a_latency_status} |
| Low-confidence destructive actions ratio | <= 0.10 | ${LOW_CONFIDENCE_RATIO} | ${slo_a_precision_status} |
| Restart-storm 10m bucket violations | 0 | ${RESTART_STORM_VIOLATIONS} | ${slo_b_storm_status} |
| API probe failures (\`/healthz,/readyz,/metrics,/timeline,/v1/ops/controlplane/replay/history\`) | 0 | ${probe_failures} | ${slo_c_api_status} |
| Latest event age | <= 300s | ${LATEST_EVENT_AGE_SECONDS}s | ${slo_c_freshness_status} |
| Idempotency conflict events | 0 | ${IDEMPOTENT_CONFLICT_EVENTS} | ${slo_c_idempotency_status} |
| Replay ledger rows | <= ${REPLAY_MAX_ROWS_TARGET_LABEL} | ${REPLAY_ROW_COUNT} | ${slo_c_replay_capacity_status} |
| Replay/conflict daily spike | yellow>=r${REPLAY_SPIKE_YELLOW}/c${CONFLICT_SPIKE_YELLOW}, red>=r${REPLAY_SPIKE_RED}/c${CONFLICT_SPIKE_RED} | ${LATEST_REPLAY_EVENTS}/${LATEST_CONFLICT_EVENTS} (${LATEST_REPLAY_DAY}) | ${slo_c_replay_spike_status} (${replay_spike_budget_trigger}) |

## Error Budget Decision

- Current status: **${ERROR_BUDGET_STATUS}**
- Warning checks: **${warn_count}**
- Failing checks: **${fail_count}**
- Replay spike trigger: **${replay_spike_budget_trigger}**

Policy:
- GREEN: continue planned roadmap work.
- YELLOW: prioritize reliability fixes; defer non-critical expansion this week.
- RED: feature freeze and start reliability sprint until status returns to GREEN.

## Replay Trend Analysis

- Latest day in window: ${LATEST_REPLAY_DAY}
- Replay events (latest day): ${LATEST_REPLAY_EVENTS}
- Conflict events (latest day): ${LATEST_CONFLICT_EVENTS}
- Previous replay peak (excluding latest day): ${PREV_REPLAY_PEAK}
- Previous conflict peak (excluding latest day): ${PREV_CONFLICT_PEAK}
- Replay ratio vs previous peak: ${REPLAY_RATIO_TO_PREV_PEAK}
- Conflict ratio vs previous peak: ${CONFLICT_RATIO_TO_PREV_PEAK}
- Spike assessment: ${REPLAY_SPIKE_REASON}

${REPLAY_TREND_MARKDOWN_TABLE}

## Activity Snapshot

- Total events: ${TOTAL_EVENTS}
- Distinct incidents: ${DISTINCT_INCIDENTS}
- Idempotent replay events: ${IDEMPOTENT_REPLAY_EVENTS}
- Idempotent conflict events: ${IDEMPOTENT_CONFLICT_EVENTS}
- Replay ledger rows (DB): ${REPLAY_ROW_COUNT}
- Replay ledger oldest age (DB): ${REPLAY_OLDEST_AGE_SECONDS}s
- Incident events: ${INCIDENT_EVENTS}
- Decision events: ${DECISION_EVENTS}
- Audit events: ${AUDIT_EVENTS}
- Policy dry-run events: ${POLICY_DRY_RUN_EVENTS}
- Destructive actions: ${DESTRUCTIVE_ACTIONS}
- Detection latency samples: ${LATENCY_SAMPLE_COUNT}
- Detection latency avg: ${LATENCY_AVG}
- FlowForge uptime (probe time): ${UPTIME_SECONDS}
- Process kill total (probe time): ${KILL_TOTAL}
- Process restart total (probe time): ${RESTART_TOTAL}
- Control-plane replay total (probe time): ${IDEMPOTENT_REPLAY_TOTAL}
- Control-plane conflict total (probe time): ${IDEMPOTENT_CONFLICT_TOTAL}
- Control-plane replay rows (probe time): ${REPLAY_ROWS_GAUGE}
- Control-plane replay oldest age (probe time): ${REPLAY_OLDEST_AGE_GAUGE}
- Replay spike status: ${slo_c_replay_spike_status} (${replay_spike_budget_trigger})

## Weekly Ritual Checklist

- [ ] Reliability lead reviewed this report in weekly ops meeting.
- [ ] Top 3 reliability risks identified and owners assigned.
- [ ] Error budget decision communicated to product/engineering.
- [ ] Corrective tasks added with due dates.
- [ ] Follow-up evidence linked in next week's report.

## Artifacts

- Summary TSV: \`${OUT_DIR}/summary.tsv\`
- Replay trend TSV: \`${OUT_DIR}/replay_daily_trend.tsv\`
- API probes:
  - \`${OUT_DIR}/healthz.json\`
  - \`${OUT_DIR}/readyz.json\`
  - \`${OUT_DIR}/metrics.prom\`
  - \`${OUT_DIR}/timeline.json\`
  - \`${OUT_DIR}/replay_history.json\`
EOF

echo "Weekly SLO review written:"
echo "- $OUT_DIR/slo_weekly_report.md"
echo "- $OUT_DIR/summary.tsv"
echo "- $OUT_DIR/replay_daily_trend.tsv"

if [[ "$FAIL_ON_BREACH" -eq 1 ]] && [[ "$ERROR_BUDGET_STATUS" == "RED" ]]; then
  echo "Error budget status is RED (fail-on-breach enabled)." >&2
  exit 1
fi
