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
for provider_name in "${providers[@]}"; do
  if [[ "$provider_name" == "openai" ]]; then
    : "${OPENAI_API_KEY:?OPENAI_API_KEY is required}"
  else
    : "${OPENROUTER_API_KEY:?OPENROUTER_API_KEY is required}"
  fi
done

"$project_root/scripts/download-test-documents.sh"

test_root="$(mktemp -d /tmp/docket-live.XXXXXX)"
trap 'rm -rf "$test_root"' EXIT
binary="$test_root/docket"
(cd "$project_root" && go build -o "$binary" ./cmd/docket)

failures=0
cases_run=0
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

  while IFS=$'\t' read -r relative_path expected; do
    [[ -z "$relative_path" || "$relative_path" == \#* ]] && continue
    if [[ -n "$case_filter" && "$relative_path" != *"$case_filter"* ]]; then
      continue
    fi
    cases_run=$((cases_run + 1))
    document="$project_root/testdata/$relative_path"
    if ! output="$(DOCKET_HOME="$state" "$binary" add "$document")"; then
      printf 'ERROR  %-34s request failed\n' "$(basename "$document")"
      failures=$((failures + 1))
      continue
    fi
    category="${output%% *}"
    if [[ "|$expected|" == *"|$category|"* ]]; then
      printf 'PASS   %-34s %s\n' "$(basename "$document")" "$output"
    else
      printf 'FAIL   %-34s expected %s, got %s\n' "$(basename "$document")" "$expected" "$output"
      failures=$((failures + 1))
    fi
  done < "$project_root/testdata/cases.tsv"
done

if (( cases_run == 0 )); then
  printf 'no live evaluation cases matched %q\n' "$case_filter" >&2
  exit 2
fi
if (( failures > 0 )); then
  printf '\n%d live evaluation failure(s)\n' "$failures" >&2
  exit 1
fi
printf '\nAll live evaluations passed.\n'
