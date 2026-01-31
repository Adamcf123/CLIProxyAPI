#!/usr/bin/env bash

set -euo pipefail

die() {
  echo "ERROR: $*" >&2
  exit 1
}

note() {
  echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] $*" >&2
}

require_cmd() {
  local cmd
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || die "missing required command: ${cmd}"
  done
}

script_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")" && pwd
}

phase_dir() {
  cd "$(script_dir)/.." && pwd
}

repo_root() {
  local phase
  phase="$(phase_dir)"
  git -C "$phase" rev-parse --show-toplevel
}

mk_run_dir() {
  local label ts safe_label root out
  label="${1:-run}"
  ts="$(date -u +"%Y%m%d-%H%M%S")"
  safe_label="$(printf '%s' "$label" | tr -cd 'A-Za-z0-9._-')"
  if [[ -z "$safe_label" ]]; then
    safe_label="run"
  fi
  root="$(phase_dir)"
  out="$root/artifacts/run-${ts}-${safe_label}"
  mkdir -p "$out"
  printf '%s\n' "$out"
}

check_port_free() {
  local port
  port="$1"

  if command -v ss >/dev/null 2>&1; then
    if ss -ltn "sport = :${port}" | rg -q "LISTEN"; then
      die "port ${port} is already in use (try: PORT=<new> or --port <new>)"
    fi
    return 0
  fi

  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
      die "port ${port} is already in use (try: PORT=<new> or --port <new>)"
    fi
    return 0
  fi

  die "cannot check port usage: need ss or lsof"
}

build_server() {
  local run_dir root out
  run_dir="$1"
  root="$(repo_root)"
  out="$run_dir/cli-proxy-api"
  (cd "$root" && go build -o "$out" ./cmd/server)
  printf '%s\n' "$out"
}

start_server() {
  local run_dir bin config port
  run_dir="$1"
  bin="$2"
  config="$3"
  port="$4"

  local stdout stderr pid_file
  stdout="$run_dir/server.stdout.log"
  stderr="$run_dir/server.stderr.log"
  pid_file="$run_dir/server.pid"

  note "starting server (cwd=${run_dir})"
  (cd "$run_dir" && "$bin" -config "$config") >"$stdout" 2>"$stderr" &
  echo "$!" >"$pid_file"

  wait_for_listen "$port" 20
}

wait_for_listen() {
  local port timeout_s start
  port="$1"
  timeout_s="$2"
  start="$(date +%s)"

  while true; do
    if command -v ss >/dev/null 2>&1; then
      if ss -ltn "sport = :${port}" | rg -q "LISTEN"; then
        return 0
      fi
    elif command -v lsof >/dev/null 2>&1; then
      if lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
        return 0
      fi
    fi

    if (( $(date +%s) - start > timeout_s )); then
      die "server did not start listening on port ${port} within ${timeout_s}s"
    fi
    sleep 0.2
  done
}

stop_server() {
  local run_dir pid_file pid
  run_dir="$1"
  pid_file="$run_dir/server.pid"
  if [[ ! -f "$pid_file" ]]; then
    note "no pid file found; nothing to stop"
    return 0
  fi
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  if [[ -z "$pid" ]]; then
    note "empty pid file; nothing to stop"
    return 0
  fi
  if ! kill -0 "$pid" >/dev/null 2>&1; then
    note "process ${pid} already stopped"
    return 0
  fi

  note "stopping server pid=${pid}"
  kill -TERM "$pid" >/dev/null 2>&1 || true

  local start timeout_s
  timeout_s=8
  start="$(date +%s)"
  while kill -0 "$pid" >/dev/null 2>&1; do
    if (( $(date +%s) - start > timeout_s )); then
      note "server did not exit after SIGTERM; sending SIGKILL"
      kill -KILL "$pid" >/dev/null 2>&1 || true
      break
    fi
    sleep 0.2
  done
}

start_resource_sampler() {
  local pid run_dir out
  pid="$1"
  run_dir="$2"
  out="$run_dir/server_resources.tsv"

  printf 'ts\tpcpu\tpmem\trss\tvsz\tetime\n' >"$out"
  (
    while kill -0 "$pid" >/dev/null 2>&1; do
      # pcpu/pmem are floats; rss/vsz are KB.
      ps -p "$pid" -o %cpu,%mem,rss,vsz,etime --no-headers \
        | awk -v ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")" '{print ts"\t"$1"\t"$2"\t"$3"\t"$4"\t"$5}' \
        >>"$out" || true
      sleep 1
    done
  ) &
  echo "$!" >"$run_dir/resource_sampler.pid"
}

stop_resource_sampler() {
  local run_dir pid
  run_dir="$1"
  if [[ -f "$run_dir/resource_sampler.pid" ]]; then
    pid="$(cat "$run_dir/resource_sampler.pid" 2>/dev/null || true)"
    if [[ -n "$pid" ]]; then
      kill -TERM "$pid" >/dev/null 2>&1 || true
    fi
  fi
}

rg_secrets_guard() {
  local target
  target="$1"

  if [[ -z "$target" ]]; then
    die "rg_secrets_guard requires a target directory"
  fi
  if [[ ! -d "$target" ]]; then
    die "rg_secrets_guard target must be a directory: ${target}"
  fi

  # Auditable output: always write a scan report under the run_dir.
  local out
  out="$target/secrets_guard_scan.txt"

  # Scan only text-like artifacts to avoid binary DB/WAL noise.
  # Keep the list aligned with Phase 11 artifact types.
  local include_globs
  include_globs=(
    "*.log"
    "*.txt"
    "*.tsv"
    "*.json"
    "*.md"
    "*.out"
    "*.sse"
  )

  # Guardrail: never persist raw auth headers; avoid false positives for placeholders like sk-dummy.
  # - Header lines must be anchored to avoid matching docs/explanations.
  # - API key pattern is length-based to avoid matching sk-dummy.
  local pattern
  pattern='(^Authorization:[[:space:]]|^X-Management-Key:[[:space:]]|sk-[A-Za-z0-9]{16,})'

  {
    echo "secrets_guard"
    echo "started_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "target=${target}"
    echo ""
    echo "scan_mode="
    echo "- rg --no-ignore (do not respect .gitignore/.ignore)"
    echo "- include globs (text-like only):"
    local g
    for g in "${include_globs[@]}"; do
      echo "  - ${g}"
    done
    echo "- exclude globs:"
    echo "  - secrets_guard_scan.txt"
    echo "  - logs/metrics.db*"
    echo "- patterns:"
    echo "  - ^Authorization:"
    echo "  - ^X-Management-Key:"
    echo "  - sk-[A-Za-z0-9]{16,}"
    echo ""
    echo "command="
    echo "rg --no-ignore --hidden --json --line-number --with-filename --color never [globs...] '${pattern}' '${target}'"
    echo ""
  } >"$out"

  local rg_args
  rg_args=(
    --no-ignore
    --hidden
    --json
    --line-number
    --with-filename
    --color
    never
    --glob
    '!secrets_guard_scan.txt'
    --glob
    '!logs/metrics.db*'
  )
  local inc
  for inc in "${include_globs[@]}"; do
    rg_args+=(--glob "$inc")
  done

  set +e
  local rg_out
  rg_out="$(rg "${rg_args[@]}" "$pattern" "$target" 2>>"$out")"
  local rg_rc=$?
  set -e

  if [[ $rg_rc -eq 2 ]]; then
    {
      echo "result=ERROR"
      echo "error=rg exited with status 2 (see stderr above)"
    } >>"$out"
    die "secrets guard scan failed (rg error); see: ${out}"
  fi

  if [[ $rg_rc -eq 0 ]]; then
    # Matches found. For safety, persist ONLY locations (path:line) instead of raw matching lines.
    local locations
    locations="$(printf '%s\n' "$rg_out" \
      | jq -r -s 'map(select(.type=="match") | (.data.path.text + ":" + (.data.line_number|tostring))) | unique | .[]' \
      2>/dev/null || true)"

    {
      echo "result=FAIL"
      echo "match_locations="
      if [[ -n "$locations" ]]; then
        printf '%s\n' "$locations"
      else
        echo "(unable to parse rg --json output; see raw output above)"
      fi
    } >>"$out"

    if [[ -n "$locations" ]]; then
      printf '%s\n' "$locations" >&2
    fi
    die "secret-like material detected under: ${target} (see ${out})"
  fi

  {
    echo "result=PASS"
    echo "note=No secret-like patterns detected in included artifact globs"
  } >>"$out"
  note "secrets_guard_scan=${out}"
}
