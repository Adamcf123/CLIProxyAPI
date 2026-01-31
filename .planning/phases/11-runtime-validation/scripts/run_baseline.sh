#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
  cat <<'EOF'
Phase 11 baseline runtime validation

Purpose:
  - Start an isolated CLIProxyAPI server instance (CWD = run_dir) so logs/metrics.db are contained
  - Run a light steady-state load (concurrency 1-5, 50/50 streaming vs non-streaming)
  - Capture audit-ready evidence (resources, curl timings, sqlite rows, stderr metrics_summary)

Usage:
  bash .planning/phases/11-runtime-validation/scripts/run_baseline.sh [options]

Options:
  --config <path>        Server config source (default: <repo_root>/config.yaml)
  --port <port>          Override port by deriving a temp config under run_dir
  --models <a,b,c>       Models to target (default: gpt-5.2,minimax-m2.1,glm-4.7)
  --concurrency <n>      Concurrency (1-5, default: 3; hard cap: 5)
  --duration-sec <n>     Run duration seconds (default: 30)
  --qps <n>              Total request rate limit (default: 2.0) across all workers
  --label <name>         Run label suffix for artifacts directory
  --help                 Show help

Environment:
  API_KEY                Client API key (placeholder ok, MUST NOT be committed)

Guardrails:
  - This script never writes raw Authorization/X-Management-Key headers to disk.
  - At the end it scans run_dir for secret-like patterns and fails loud if found.

Outputs (all under artifacts/run-*/):
  - server.stdout.log, server.stderr.log
  - server_resources.tsv
  - curl_timings.tsv
  - sqlite_metrics_snapshot.txt
  - run_meta.json
EOF
}

derive_config_with_port() {
  local src dst port
  src="$1"
  dst="$2"
  port="$3"

  awk -v p="$port" '
    BEGIN { done=0 }
    {
      if ($0 ~ /^[[:space:]]*port:[[:space:]]*[0-9]+([[:space:]]*#.*)?$/ && done==0) {
        sub(/port:[[:space:]]*[0-9]+/, "port: " p)
        done=1
      }
      print
    }
    END { if (done==0) exit 3 }
  ' "$src" >"$dst" || die "failed to derive config with port=${port} (ensure config has a top-level port: field)"
}

pick_endpoint() {
  local model
  model="$1"
  # Heuristic aligned with 重要命令.txt examples.
  if [[ "$model" == glm-* ]] || [[ "$model" == gpt-* ]]; then
    echo "openai"
    return
  fi
  echo "messages"
}

main() {
  local root config_src port models concurrency duration_s qps label
  root="$(repo_root)"
  config_src="$root/config.yaml"
  port="${PORT:-53355}"
  models="gpt-5.2,minimax-m2.1,glm-4.7"
  concurrency=3
  duration_s=30
  qps=2.0
  label="baseline"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config) config_src="$2"; shift 2 ;;
      --port) port="$2"; shift 2 ;;
      --models) models="$2"; shift 2 ;;
      --concurrency) concurrency="$2"; shift 2 ;;
      --duration-sec) duration_s="$2"; shift 2 ;;
      --qps) qps="$2"; shift 2 ;;
      --label) label="$2"; shift 2 ;;
      --help|-h) require_cmd go curl jq sqlite3 rg ss; usage; return 0 ;;
      *) die "unknown arg: $1 (try --help)" ;;
    esac
  done

  require_cmd go curl jq sqlite3 rg ss

  if [[ -z "${API_KEY:-}" ]]; then
    die "API_KEY is required (placeholder ok). Example: export API_KEY=sk-dummy"
  fi

  if [[ "$concurrency" -lt 1 ]]; then
    die "concurrency must be >= 1"
  fi
  if [[ "$concurrency" -gt 5 ]]; then
    die "concurrency must be <= 5 (hard cap)"
  fi

  check_port_free "$port"

  local run_dir
  run_dir="$(mk_run_dir "$label")"
  note "run_dir=${run_dir}"

  local config_run
  config_run="$run_dir/config.yaml"
  if [[ ! -f "$config_src" ]]; then
    die "config not found: ${config_src}"
  fi
  derive_config_with_port "$config_src" "$config_run" "$port"

  local bin
  bin="$(build_server "$run_dir")"

  start_server "$run_dir" "$bin" "$config_run" "$port"

  local server_pid
  server_pid="$(cat "$run_dir/server.pid")"
  start_resource_sampler "$server_pid" "$run_dir"

  local base_url
  base_url="http://127.0.0.1:${port}"

  # Wait until the HTTP stack is responsive.
  local start
  start="$(date +%s)"
  while true; do
    if curl -sS -o /dev/null "$base_url/"; then
      break
    fi
    if (( $(date +%s) - start > 10 )); then
      die "server did not respond to HTTP within 10s (see ${run_dir}/server.stderr.log)"
    fi
    sleep 0.2
  done

  # Record metadata early.
  local commit
  commit="$(git -C "$root" rev-parse HEAD)"
  jq -n \
    --arg started_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --arg git_commit "$commit" \
    --arg config_src "$config_src" \
    --arg config_run "$config_run" \
    --arg secrets_guard_scan "$run_dir/secrets_guard_scan.txt" \
    --arg port "$port" \
    --arg models "$models" \
    --arg concurrency "$concurrency" \
    --arg duration_sec "$duration_s" \
    --arg qps "$qps" \
    '{started_at:$started_at, git_commit:$git_commit, config_src:$config_src, config_run:$config_run, secrets_guard_scan:$secrets_guard_scan, port:($port|tonumber), models:($models|split(",")), concurrency:($concurrency|tonumber), duration_sec:($duration_sec|tonumber), qps:($qps|tonumber)}' \
    >"$run_dir/run_meta.json"

  local timings
  timings="$run_dir/curl_timings.tsv"
  printf 'ts\tworker\tseq\tendpoint\tmodel\tstreaming\thttp_code\tconnect\ttotal\tcurl_exit\n' >"$timings"

  local end_epoch
  end_epoch=$(( $(date +%s) + duration_s ))

  # Throttle per worker to enforce a total QPS budget.
  # Each worker sleeps ~ (concurrency / qps) seconds per request (plus small jitter).
  local sleep_s
  sleep_s="$(awk -v c="$concurrency" -v q="$qps" 'BEGIN{ if(q<=0){print 0}else{printf "%.3f", (c/q)} }')"

  note "running steady-state load: concurrency=${concurrency} duration=${duration_s}s qps=${qps} (~sleep ${sleep_s}s/req/worker)"

  local models_arr
  IFS=',' read -r -a models_arr <<<"$models"
  if [[ "${#models_arr[@]}" -lt 2 ]]; then
    die "--models must include 2-3 models (comma-separated)"
  fi

  worker() {
    local wid seq
    wid="$1"
    seq=0
    while [[ $(date +%s) -lt $end_epoch ]]; do
      seq=$((seq + 1))
      local model stream endpoint url payload curl_exit http_code connect total ts
      model="${models_arr[$(( (seq + wid) % ${#models_arr[@]} ))]}"
      endpoint="$(pick_endpoint "$model")"

      # 50/50 stream vs non-stream
      if (( (seq + wid) % 2 == 0 )); then
        stream="true"
      else
        stream="false"
      fi

      ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

      if [[ "$endpoint" == "openai" ]]; then
        url="$base_url/v1/chat/completions"
        payload="{\"model\":\"$model\",\"stream\":$stream,\"max_tokens\":64,\"messages\":[{\"role\":\"user\",\"content\":\"Say hi in one short sentence.\"}]}"
      else
        url="$base_url/v1/messages"
        payload="{\"model\":\"$model\",\"stream\":$stream,\"max_tokens\":64,\"messages\":[{\"role\":\"user\",\"content\":\"Write a one-line hello world program in Go.\"}]}"
      fi

      # Never print headers; only record timing metrics.
      local curl_stream_arg
      curl_stream_arg=""
      if [[ "$stream" == "true" ]]; then
        curl_stream_arg="-N"
      fi
      set +e
      read -r http_code connect total < <(
        curl -sS $curl_stream_arg -o /dev/null \
          -H "Content-Type: application/json" \
          -H "Authorization: Bearer ${API_KEY}" \
          -X POST "$url" \
          --data-binary "$payload" \
          -w "%{http_code} %{time_connect} %{time_total}" \
        2>/dev/null
      )
      curl_exit=$?
      set -e

      printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
        "$ts" "$wid" "$seq" "$endpoint" "$model" "$stream" "$http_code" "$connect" "$total" "$curl_exit" \
        >>"$timings"

      # jitter up to 150ms to avoid perfect alignment
      if [[ "$sleep_s" != "0" ]]; then
        sleep "$sleep_s"
      fi
      sleep "0.$((RANDOM % 150))" || true
    done
  }

  local pids=()
  local i
  for ((i=1; i<=concurrency; i++)); do
    worker "$i" &
    pids+=("$!")
  done
  for pid in "${pids[@]}"; do
    wait "$pid" || true
  done

  stop_resource_sampler "$run_dir"
  stop_server "$run_dir"

  # SQLite snapshot (best-effort; the DB should live under run_dir/logs/metrics.db).
  local db
  db="$run_dir/logs/metrics.db"
  if [[ -f "$db" ]]; then
    {
      echo "-- sqlite metrics snapshot"
      echo "-- db: $db"
      sqlite3 "$db" "SELECT COUNT(*) AS total_rows FROM metrics;"
      echo
      sqlite3 "$db" "SELECT request_id, provider, model, streaming, status_code, error_info FROM metrics ORDER BY created_at DESC LIMIT 5;"
    } >"$run_dir/sqlite_metrics_snapshot.txt" || true
  else
    echo "metrics db not found at: $db" >"$run_dir/sqlite_metrics_snapshot.txt"
  fi

  # Capture some metrics_summary lines for later correlation.
  if [[ -f "$run_dir/server.stderr.log" ]]; then
    rg "^metrics_summary " "$run_dir/server.stderr.log" \
      | awk 'NR<=10{print} END{if(NR==0)exit 0}' \
      >"$run_dir/metrics_summary_sample.txt" || true
  fi

  rg_secrets_guard "$run_dir"

  note "baseline run complete"
  note "artifacts: ${run_dir}"
}

main "$@"
