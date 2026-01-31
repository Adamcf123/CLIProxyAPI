#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
Phase 11 edge-case runtime validation

Purpose:
  Script and capture evidence for required edge scenarios:
    - terminal-error-after-headers
    - client-cancel
    - no-usage
    - persistence-degraded

Usage:
  bash .planning/phases/11-runtime-validation/scripts/run_edge_cases.sh <command> [options]

Commands:
  terminal-error-after-headers
  client-cancel
  no-usage
  persistence-degraded
  all

Options:
  --repeats <n>          Repeat each scenario (default: 3)
  --port <port>          Proxy port (default: 53356)
  --upstream-port <port> Mock upstream port (default: 53357)
  --label <name>         Run label suffix for artifacts directory
  --help                 Show help

Environment:
  MANAGEMENT_KEY         Validation-only plaintext management key (default suggestion: phase11-dev)
  API_KEY                Client API key (default: sk-dummy)

Evidence:
  A single run_dir is created under artifacts/ and contains:
    - server stdout/stderr logs
    - SQLite metrics db (under run_dir/logs/metrics.db)
    - edge_evidence.tsv (request_id = metrics_summary.tracking_id)
    - management_metrics_*.json (persistence degraded evidence)

Security:
  This script does NOT persist raw auth headers. It performs a secret-pattern scan at end.
EOF
}

write_temp_config() {
  local out port upstream_port management_key
  out="$1"
  port="$2"
  upstream_port="$3"
  management_key="$4"

  cat >"$out" <<EOF
host: "127.0.0.1"
port: ${port}

# Ensure the proxy can always create its auth directory when running from an isolated run_dir.
# The server process CWD is the run_dir, so a relative path keeps all artifacts self-contained.
auth-dir: "./auth"

remote-management:
  allow-remote: false
  secret-key: "${management_key}"

api-keys:
  - "sk-dummy"

openai-compatibility:
  - name: "mock-openai"
    base-url: "http://127.0.0.1:${upstream_port}"
    api-key-entries:
      - api-key: "sk-upstream-dummy"
    models:
      - name: "mock-stream"
        alias: "mock-stream"
EOF
}

extract_last_request_id() {
  local stderr_file line json request_id
  stderr_file="$1"
  line="$(rg '^metrics_summary ' "$stderr_file" | awk 'END{print}')"
  if [[ -z "$line" ]]; then
    die "no metrics_summary line found in: ${stderr_file}"
  fi
  json="${line#metrics_summary }"
  request_id="$(printf '%s' "$json" | jq -r '.tracking_id')"
  if [[ -z "$request_id" || "$request_id" == "null" ]]; then
    die "failed to parse tracking_id from metrics_summary"
  fi
  if ! [[ "$request_id" =~ ^[0-9a-f]{16}$ ]]; then
    die "unexpected request_id format (expected 16-char hex): ${request_id}"
  fi
  printf '%s\n' "$request_id"
}

sqlite_row() {
  local db request_id
  db="$1"
  request_id="$2"
  sqlite3 -separator $'\t' "$db" "SELECT request_id, status_code, error_info, input_tokens, output_tokens, streaming FROM metrics WHERE request_id='${request_id}' LIMIT 1;"
}

append_evidence() {
  local file scenario iter request_id expected observed artifact
  file="$1"
  scenario="$2"
  iter="$3"
  request_id="$4"
  expected="$5"
  observed="$6"
  artifact="$7"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$scenario" "$iter" "$request_id" "$expected" "$observed" "$artifact" \
    >>"$file"
}

start_mock_upstream() {
  local run_dir upstream_port root pid_file
  run_dir="$1"
  upstream_port="$2"
  root="$(repo_root)"
  pid_file="$run_dir/mock_upstream.pid"

  note "starting mock upstream on port ${upstream_port}"
  (cd "$root" && PORT="$upstream_port" go run .planning/phases/11-runtime-validation/tools/mock_openai_compat_upstream.go) \
    >"$run_dir/mock_upstream.stdout.log" 2>"$run_dir/mock_upstream.stderr.log" &
  echo "$!" >"$pid_file"
  wait_for_listen "$upstream_port" 10
}

stop_mock_upstream() {
  local run_dir pid
  run_dir="$1"
  if [[ -f "$run_dir/mock_upstream.pid" ]]; then
    pid="$(cat "$run_dir/mock_upstream.pid" 2>/dev/null || true)"
    if [[ -n "$pid" ]]; then
      kill -TERM "$pid" >/dev/null 2>&1 || true
    fi
  fi
}

scenario_terminal_error_after_headers() {
  local base_url db stderr evidence
  base_url="$1"
  db="$2"
  stderr="$3"
  evidence="$4"
  local i
  for ((i=1; i<=REPEATS; i++)); do
    set +e
    curl -sS -N -o "$RUN_DIR/terminal_error_${i}.sse" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer ${API_KEY}" \
      -X POST "$base_url/v1/chat/completions" \
      --data-binary '{"model":"mock-stream","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"stream then upstream disconnects"}]}' \
      >/dev/null 2>&1
    set -e

    local request_id row status_code error_info observed
    request_id="$(extract_last_request_id "$stderr")"
    row="$(sqlite_row "$db" "$request_id" || true)"
    if [[ -z "$row" ]]; then
      append_evidence "$evidence" "terminal-error-after-headers" "$i" "$request_id" "db_row_present_and_failure_semantics" "db_row_missing" "$RUN_DIR/server.stderr.log"
      die "expected sqlite row for request_id=${request_id}, but none found"
    fi

    status_code="$(printf '%s' "$row" | awk -F'\t' '{print $2}')"
    error_info="$(printf '%s' "$row" | awk -F'\t' '{print $3}')"

    observed="status=${status_code} error_info_len=${#error_info}"
    if [[ "$status_code" =~ ^2[0-9][0-9]$ ]] && [[ -z "$error_info" ]]; then
      append_evidence "$evidence" "terminal-error-after-headers" "$i" "$request_id" "failure" "unexpected_success" "$RUN_DIR/sqlite_check_terminal_error_${i}.tsv"
      die "expected failure semantics (non-2xx or error_info) for request_id=${request_id}, got 2xx with empty error_info"
    fi

    printf '%s\n' "$row" >"$RUN_DIR/sqlite_check_terminal_error_${i}.tsv"
    append_evidence "$evidence" "terminal-error-after-headers" "$i" "$request_id" "failure" "$observed" "$RUN_DIR/sqlite_check_terminal_error_${i}.tsv"
  done
}

scenario_client_cancel() {
  local base_url db stderr evidence root
  base_url="$1"
  db="$2"
  stderr="$3"
  evidence="$4"
  root="$(repo_root)"

  local i
  for ((i=1; i<=REPEATS; i++)); do
    (cd "$root" && API_KEY="$API_KEY" go run .planning/phases/11-runtime-validation/tools/cancel_stream_client.go \
      --url "$base_url" --model mock-stream --cancel-after-bytes 256) \
      >"$RUN_DIR/client_cancel_${i}.out" 2>"$RUN_DIR/client_cancel_${i}.err" || true

    local request_id row status_code error_info observed
    request_id="$(extract_last_request_id "$stderr")"
    row="$(sqlite_row "$db" "$request_id" || true)"
    if [[ -z "$row" ]]; then
      append_evidence "$evidence" "client-cancel" "$i" "$request_id" "status_code=499 and empty_error" "db_row_missing" "$RUN_DIR/server.stderr.log"
      die "expected sqlite row for request_id=${request_id}, but none found"
    fi
    status_code="$(printf '%s' "$row" | awk -F'\t' '{print $2}')"
    error_info="$(printf '%s' "$row" | awk -F'\t' '{print $3}')"
    observed="status=${status_code} error_info_len=${#error_info}"
    printf '%s\n' "$row" >"$RUN_DIR/sqlite_check_client_cancel_${i}.tsv"

    if [[ "$status_code" != "499" ]]; then
      append_evidence "$evidence" "client-cancel" "$i" "$request_id" "499" "$observed" "$RUN_DIR/sqlite_check_client_cancel_${i}.tsv"
      die "expected status_code=499 for request_id=${request_id}, got ${status_code}"
    fi
    if [[ -n "$error_info" ]]; then
      append_evidence "$evidence" "client-cancel" "$i" "$request_id" "empty_error" "$observed" "$RUN_DIR/sqlite_check_client_cancel_${i}.tsv"
      die "expected empty error_info for canceled request_id=${request_id}, got non-empty"
    fi

    append_evidence "$evidence" "client-cancel" "$i" "$request_id" "499+empty_error" "$observed" "$RUN_DIR/sqlite_check_client_cancel_${i}.tsv"
  done
}

scenario_no_usage() {
  local base_url db stderr evidence
  base_url="$1"
  db="$2"
  stderr="$3"
  evidence="$4"

  local i
  for ((i=1; i<=REPEATS; i++)); do
    # Use streaming mode so stderr emits a metrics_summary line we can correlate.
    curl -sS -N -o "$RUN_DIR/no_usage_${i}.sse" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer ${API_KEY}" \
      -X POST "$base_url/v1/chat/completions" \
      --data-binary '{"model":"mock-stream","stream":true,"max_tokens":8,"messages":[{"role":"user","content":"no usage please"}]}'

    local request_id row usage_note observed
    request_id="$(extract_last_request_id "$stderr")"
    row="$(sqlite_row "$db" "$request_id" || true)"
    if [[ -z "$row" ]]; then
      append_evidence "$evidence" "no-usage" "$i" "$request_id" "db_row_present" "db_row_missing" "$RUN_DIR/server.stderr.log"
      die "expected sqlite row for request_id=${request_id}, but none found"
    fi

    usage_note="$(rg '^metrics_summary ' "$stderr" | awk 'END{print}' | sed 's/^metrics_summary //' | jq -r '.usage_note')"
    observed="usage_note=${usage_note}"

    printf '%s\n' "$row" >"$RUN_DIR/sqlite_check_no_usage_${i}.tsv"
    append_evidence "$evidence" "no-usage" "$i" "$request_id" "row_present" "$observed" "$RUN_DIR/sqlite_check_no_usage_${i}.tsv"
  done
}

scenario_persistence_degraded() {
  local base_url db evidence
  base_url="$1"
  db="$2"
  evidence="$3"

  # Hold an EXCLUSIVE lock using sqlite3 over a FIFO so the lock is deterministic and releasable.
  local fifo lock_pid
  fifo="$RUN_DIR/sqlite_lock.fifo"
  mkfifo "$fifo"
  sqlite3 "$db" <"$fifo" >"$RUN_DIR/sqlite_lock.stdout.log" 2>"$RUN_DIR/sqlite_lock.stderr.log" &
  lock_pid=$!
  exec 9>"$fifo"
  printf 'BEGIN EXCLUSIVE;\n' >&9

  # Flood the request path while holding the DB lock. This is intended to
  # deterministically trigger best-effort drops (queue_full and/or insert_failure)
  # and therefore surface meta.persistence.degraded=true.
  local burst
  burst=1100
  local i
  for ((i=1; i<=REPEATS; i++)); do
    seq 1 "$burst" \
      | xargs -P 24 -I{} \
        curl -sS -o /dev/null \
          -H "Content-Type: application/json" \
          -H "Authorization: Bearer ${API_KEY}" \
          -X POST "$base_url/v1/chat/completions" \
          --data-binary '{"model":"mock-stream","stream":false,"max_tokens":1,"messages":[{"role":"user","content":"trigger persistence degraded"}]}' \
        || true

    # Query management endpoint for degraded meta.
    local meta_file
    meta_file="$RUN_DIR/management_metrics_persistence_degraded_${i}.json"
    curl -sS \
      -H "X-Management-Key: ${MANAGEMENT_KEY}" \
      "$base_url/v0/management/metrics?mode=percentiles" \
      >"$meta_file" || true

    local degraded reason dropped_total
    degraded="$(jq -r '.meta.persistence.degraded // false' "$meta_file" 2>/dev/null || echo false)"
    reason="$(jq -r '.meta.persistence.last_drop_reason // ""' "$meta_file" 2>/dev/null || echo "")"
    dropped_total="$(jq -r '.meta.persistence.dropped_total // 0' "$meta_file" 2>/dev/null || echo 0)"
    append_evidence "$evidence" "persistence-degraded" "$i" "--" "meta.persistence.degraded=true" "degraded=${degraded} dropped_total=${dropped_total} reason=${reason}" "$meta_file"
  done

  # Release lock.
  printf 'COMMIT;\n' >&9
  exec 9>&-
  wait "$lock_pid" || true
  rm -f "$fifo" || true
}

main() {
  local cmd
  cmd="${1:-}"

  local port upstream_port label
  REPEATS=3
  port=53356
  upstream_port=53357
  label="edge"

  if [[ "$cmd" == "--help" || "$cmd" == "-h" || -z "$cmd" ]]; then
    require_cmd go curl jq sqlite3 rg ss
    usage
    return 0
  fi
  shift 1

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repeats) REPEATS="$2"; shift 2 ;;
      --port) port="$2"; shift 2 ;;
      --upstream-port) upstream_port="$2"; shift 2 ;;
      --label) label="$2"; shift 2 ;;
      --help|-h) require_cmd go curl jq sqlite3 rg ss; usage; return 0 ;;
      *) die "unknown arg: $1 (try --help)" ;;
    esac
  done

  require_cmd go curl jq sqlite3 rg ss

  MANAGEMENT_KEY="${MANAGEMENT_KEY:-phase11-dev}"
  API_KEY="${API_KEY:-sk-dummy}"

  if [[ "$REPEATS" -lt 3 ]]; then
    die "repeats must be >= 3 (context requirement)"
  fi

  check_port_free "$port"
  check_port_free "$upstream_port"

  RUN_DIR="$(mk_run_dir "$label")"
  note "run_dir=${RUN_DIR}"

  local cfg
  cfg="$RUN_DIR/config.yaml"
  write_temp_config "$cfg" "$port" "$upstream_port" "$MANAGEMENT_KEY"

  local bin
  bin="$(build_server "$RUN_DIR")"

  start_mock_upstream "$RUN_DIR" "$upstream_port"
  start_server "$RUN_DIR" "$bin" "$cfg" "$port"

  local server_pid
  server_pid="$(cat "$RUN_DIR/server.pid")"
  start_resource_sampler "$server_pid" "$RUN_DIR"

  local base_url db evidence
  base_url="http://127.0.0.1:${port}"
  db="$RUN_DIR/logs/metrics.db"
  evidence="$RUN_DIR/edge_evidence.tsv"
  printf 'ts\tscenario\titer\trequest_id\texpected\tobserved\tartifact\n' >"$evidence"

  # Wait for DB to exist.
  local start
  start="$(date +%s)"
  while [[ ! -f "$db" ]]; do
    if (( $(date +%s) - start > 10 )); then
      die "metrics db did not appear within 10s (see ${RUN_DIR}/server.stderr.log)"
    fi
    sleep 0.2
  done

  case "$cmd" in
    terminal-error-after-headers)
      scenario_terminal_error_after_headers "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      ;;
    client-cancel)
      scenario_client_cancel "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      ;;
    no-usage)
      scenario_no_usage "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      ;;
    persistence-degraded)
      scenario_persistence_degraded "$base_url" "$db" "$evidence"
      ;;
    all)
      scenario_terminal_error_after_headers "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      scenario_client_cancel "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      scenario_no_usage "$base_url" "$db" "$RUN_DIR/server.stderr.log" "$evidence"
      scenario_persistence_degraded "$base_url" "$db" "$evidence"
      ;;
    *)
      die "unknown command: ${cmd} (try --help)"
      ;;
  esac

  stop_resource_sampler "$RUN_DIR"
  stop_server "$RUN_DIR"
  stop_mock_upstream "$RUN_DIR"

  rg_secrets_guard "$RUN_DIR"
  note "edge-case run complete"
  note "artifacts: ${RUN_DIR}"
}

main "$@"
