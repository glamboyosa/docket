#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
provider_arg="${1:-all}"
case_filter="${DOCKET_TEST_FILTER:-}"
case "$provider_arg" in
  openai) providers=(openai) ;;
  openrouter) providers=(openrouter) ;;
  all) providers=(openai openrouter) ;;
  *) printf 'usage: %s openai|openrouter|all\n' "$0" >&2; exit 2 ;;
esac

if [[ -f "$project_root/.env" ]]; then
  set -a
  source "$project_root/.env"
  set +a
fi

: "${TYPESAFE_API_KEY:?TYPESAFE_API_KEY is required}"
export LANGFUSE_TRACING_ENVIRONMENT=experiment
for provider_name in "${providers[@]}"; do
  if [[ "$provider_name" == "openai" ]]; then
    : "${OPENAI_API_KEY:?OPENAI_API_KEY is required}"
  else
    : "${OPENROUTER_API_KEY:?OPENROUTER_API_KEY is required}"
  fi
done

needs_download=false
while IFS=$'\t' read -r relative_path _; do
  if [[ "$relative_path" == documents/downloaded/* && ( -z "$case_filter" || "$relative_path" == *"$case_filter"* ) ]]; then
    needs_download=true
    break
  fi
done < "$project_root/testdata/cases.tsv"
if [[ "$needs_download" == true ]]; then
  "$project_root/scripts/download-test-documents.sh"
fi

test_root="$(mktemp -d /tmp/docket-live.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
binary="$test_root/docket"
(cd "$project_root" && go build -o "$binary" ./cmd/docket)

failures=0
cases_run=0
category_passes=0
action_passes=0
action_cases=0
for provider_name in "${providers[@]}"; do
  state="$test_root/$provider_name/state"
  library="$test_root/$provider_name/library"
  if [[ "$provider_name" == "openai" ]]; then
    model="${DOCKET_TEST_OPENAI_MODEL:-gpt-4.1-mini}"
  else
    model="${DOCKET_TEST_OPENROUTER_MODEL:-openrouter/auto}"
  fi
  DOCKET_HOME="$state" "$binary" config provider "$provider_name"
  DOCKET_HOME="$state" "$binary" config model "$model"
  DOCKET_HOME="$state" "$binary" config library "$library"
  printf '\n%s / %s\n' "$provider_name" "$model"
  trace_file="$test_root/$provider_name/trace-id"

  while IFS=$'\t' read -r relative_path expected expected_action; do
    [[ -z "$relative_path" || "$relative_path" == \#* ]] && continue
    if [[ -n "$case_filter" && "$relative_path" != *"$case_filter"* ]]; then
      continue
    fi
    cases_run=$((cases_run + 1))
    document="$project_root/testdata/$relative_path"
    if ! output="$(DOCKET_HOME="$state" DOCKET_EVAL_TRACE_ID_FILE="$trace_file" "$binary" add "$document")"; then
      printf 'ERROR  %-34s request failed\n' "$(basename "$document")"
      failures=$((failures + 1))
      continue
    fi
    category="${output%% *}"
    category_match=0
    if [[ "|$expected|" == *"|$category|"* ]]; then
      printf 'PASS   %-34s %s\n' "$(basename "$document")" "$output"
      category_passes=$((category_passes + 1))
      category_match=1
    else
      printf 'FAIL   %-34s expected %s, got %s\n' "$(basename "$document")" "$expected" "$output"
      failures=$((failures + 1))
    fi
    action_match=""
    if [[ "$expected_action" == "0" || "$expected_action" == "1" ]]; then
      action_cases=$((action_cases + 1))
      actual_action="$(sqlite3 "$state/docket.db" "SELECT needs_action FROM documents WHERE original_name = '$(basename "$document")' ORDER BY id DESC LIMIT 1;")"
      if [[ "$actual_action" == "$expected_action" ]]; then
        action_passes=$((action_passes + 1))
        action_match=1
      else
        printf 'FAIL   %-34s expected needs_action=%s, got %s\n' "$(basename "$document")" "$expected_action" "$actual_action"
        failures=$((failures + 1))
        action_match=0
      fi
    fi
    if [[ -n "${LANGFUSE_PUBLIC_KEY:-}" && -n "${LANGFUSE_SECRET_KEY:-}" ]]; then
      trace_id="$(cat "$trace_file")"
      base_url="${LANGFUSE_BASE_URL:-https://cloud.langfuse.com}"
      if ! curl --fail --silent --show-error --output /dev/null -u "$LANGFUSE_PUBLIC_KEY:$LANGFUSE_SECRET_KEY" \
        -H 'Content-Type: application/json' \
        --data "{\"traceId\":\"$trace_id\",\"name\":\"docket.category_accuracy\",\"value\":$category_match,\"dataType\":\"BOOLEAN\"}" \
        "$base_url/api/public/scores"; then
        printf 'FAIL   Langfuse category score upload\n' >&2
        failures=$((failures + 1))
      fi
      if [[ -n "$action_match" ]]; then
        if ! curl --fail --silent --show-error --output /dev/null -u "$LANGFUSE_PUBLIC_KEY:$LANGFUSE_SECRET_KEY" \
          -H 'Content-Type: application/json' \
          --data "{\"traceId\":\"$trace_id\",\"name\":\"docket.action_accuracy\",\"value\":$action_match,\"dataType\":\"BOOLEAN\"}" \
          "$base_url/api/public/scores"; then
          printf 'FAIL   Langfuse action score upload\n' >&2
          failures=$((failures + 1))
        fi
      fi
    fi
  done < "$project_root/testdata/cases.tsv"
done

if (( cases_run == 0 )); then
  printf 'no live evaluation cases matched %q\n' "$case_filter" >&2
  exit 2
fi
printf '\nCategory accuracy: %d/%d; action accuracy: %d/%d\n' "$category_passes" "$cases_run" "$action_passes" "$action_cases"
if (( failures > 0 )); then
  printf '\n%d live evaluation failure(s)\n' "$failures" >&2
  exit 1
fi
printf '\nAll live evaluations passed.\n'
