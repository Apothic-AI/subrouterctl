#!/usr/bin/env bash
# smoke.sh exercises every subrouterctl command group against a live account.
#
#   ./scripts/smoke.sh [binary]
#
# The script creates and then removes a few throwaway API keys, so it needs a
# real system access token. It restores the account to its original state.
set -uo pipefail

BIN="${1:-./subrouterctl}"
BASE="${SUBROUTER_BASE_URL:-https://subrouter.ai}"
PASS=0
FAIL=0
CREATED_KEYS=()

if [[ ! -x "$BIN" ]]; then
  echo "smoke: binary not found or not executable: $BIN" >&2
  exit 2
fi

# check <name> <command...>
check() {
  local name="$1"; shift
  local out
  if out=$("$@" 2>&1); then
    PASS=$((PASS + 1))
    printf '  \033[32m✓\033[0m %s\n' "$name"
  else
    FAIL=$((FAIL + 1))
    printf '  \033[31m✗\033[0m %s\n     %s\n' "$name" "$(printf '%s' "$out" | head -3 | tr '\n' ' ')"
  fi
}

# check_fail <name> <command...>  — expects a non-zero exit
check_fail() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    FAIL=$((FAIL + 1))
    printf '  \033[31m✗\033[0m %s (expected an error, got success)\n' "$name"
  else
    PASS=$((PASS + 1))
    printf '  \033[32m✓\033[0m %s\n' "$name"
  fi
}

cleanup() {
  if (( ${#CREATED_KEYS[@]} )); then
    "$BIN" token delete-batch "${CREATED_KEYS[@]}" --yes >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo
echo "subrouterctl smoke test against $BASE"
echo "───────────────────────────────────────────────────────────────────────────"

echo
echo "credentials"
check "auth status" "$BIN" auth status

echo
echo "account"
check "account info" "$BIN" account info
check "account groups" "$BIN" account groups
check "account models" "$BIN" account models
check "account subrouter-models" "$BIN" account subrouter-models
check "account topup-info" "$BIN" account topup-info
check "account topups" "$BIN" account topups
check "account balance" "$BIN" account balance
check "account notifications" "$BIN" account notifications
check "account aff" "$BIN" account aff
check "account status" "$BIN" account status

echo
echo "usage"
check "usage stats" "$BIN" usage stats
check "usage data" "$BIN" usage data --from 7d --to now
check "usage logs" "$BIN" usage logs --limit 5
check "usage logs by model" "$BIN" usage logs --model deepseek --limit 5
check "usage logs sorted by cost" "$BIN" usage logs --sort cost --order desc --limit 5
check "usage logs sorted by tps" "$BIN" usage logs --sort tps --order desc --limit 5
check "usage logs sorted by ttft" "$BIN" usage logs --sort ttft --order asc --limit 5
check "usage export url" "$BIN" usage export --from 1d --to now

request_id=$("$BIN" -o json usage logs --limit 1 2>/dev/null | grep -oE '"request_id": *"[^"]+"' | head -1 | grep -oE '"[^"]+"$' | tr -d '"')
if [[ -n "$request_id" ]]; then
  check "usage logs by request id" "$BIN" usage logs --request "$request_id" --limit 1
  check "usage log detail" "$BIN" usage log "$request_id"
  check "usage log detail json" "$BIN" -o json usage log "$request_id"
else
  FAIL=$((FAIL + 1))
  printf '  \033[31m\u2717\033[0m could not resolve a request id\n'
fi

echo
echo "token lifecycle"
for kind in smart fixed shared official; do
  args=(token create --type "$kind" --name "smoke-$kind" --unlimited)
  [[ "$kind" == fixed ]] && args+=(--provider demo)
  [[ "$kind" == smart ]] && args+=(--providers demo)
  if out=$("$BIN" "${args[@]}" 2>&1); then
    PASS=$((PASS + 1))
    printf '  \033[32m✓\033[0m create %s key\n' "$kind"
    id=$(printf '%s' "$out" | grep -oE 'id [0-9]+' | head -1 | grep -oE '[0-9]+')
    [[ -n "$id" ]] && CREATED_KEYS+=("$id")
    # The platform must classify the key under the kind we asked for.
    if "$BIN" token list --kind "$kind" --search "smoke-$kind" 2>&1 | grep -q "smoke-$kind"; then
      PASS=$((PASS + 1))
      printf '  \033[32m\u2713\033[0m %s key is classified as %s\n' "$kind" "$kind"
    else
      FAIL=$((FAIL + 1))
      printf '  \033[31m\u2717\033[0m %s key is not classified as %s\n' "$kind" "$kind"
    fi
  else
    FAIL=$((FAIL + 1))
    printf '  \033[31m✗\033[0m create %s key: %s\n' "$kind" "$(printf '%s' "$out" | head -2 | tr '\n' ' ')"
  fi
done
if (( ${#CREATED_KEYS[@]} )); then
  first="${CREATED_KEYS[0]}"
  check "token show" "$BIN" token show "$first"
  check "token update" "$BIN" token update "$first" --name "smoke-renamed"
  check "token list by kind" "$BIN" token list --kind fixed --limit 5
  check "token list search" "$BIN" token list --search smoke --limit 5
  check "token list sorted" "$BIN" token list --sort used --order desc --limit 5
  check "token types" "$BIN" token types
fi

echo
echo "model directory"
check "model list" "$BIN" model list --limit 5
check "model list search" "$BIN" model list --search gpt --limit 5
check "model list sorted by price" "$BIN" model list --sort price --order desc --limit 5
check "model routing" "$BIN" model routing --limit 5
check "model pricing" "$BIN" model pricing
check "model options" "$BIN" model options

echo
echo "single listings"
model_id=$("$BIN" -o json model list --limit 1 2>/dev/null | grep -oE '"id": *"?[0-9]+' | head -1 | grep -oE '[0-9]+')
if [[ -n "$model_id" ]]; then
  check "model show <id>" "$BIN" model show "$model_id"
  check "model show json" "$BIN" -o json model show "$model_id"
else
  FAIL=$((FAIL + 1))
  printf '  \033[31m\u2717\033[0m could not resolve a model id\n'
fi
provider_slug=$("$BIN" -o json provider list --limit 1 2>/dev/null | grep -oE '"slug": *"[^"]+"' | head -1 | grep -oE '"[^"]+"$' | tr -d '"')
if [[ -n "$provider_slug" ]]; then
  check "provider show <slug>" "$BIN" provider show "$provider_slug"
  check "provider models <slug>" "$BIN" provider models "$provider_slug"
  check "provider models searched" "$BIN" provider models "$provider_slug" --search gpt
else
  FAIL=$((FAIL + 1))
  printf '  \033[31m\u2717\033[0m could not resolve a provider slug\n'
fi

echo
echo "provider directory"
check "provider list" "$BIN" provider list --limit 5
check "provider list search" "$BIN" provider list --search demo --limit 5
check "provider list sorted" "$BIN" provider list --sort rating --order desc --limit 5
check "provider subs" "$BIN" provider subs --limit 5
check "provider sub-config" "$BIN" provider sub-config
check "provider shared" "$BIN" provider shared

echo
echo "subscriptions"
check "shared list" "$BIN" shared list
check "subscription list" "$BIN" subscription list
check "subscription plans" "$BIN" subscription plans

echo
echo "tasks"
check "task list" "$BIN" task list
check "task list filtered" "$BIN" task list --search x --limit 5

echo
echo "cache"
check "cache round trip" "$BIN" --cache-ttl 1m model list --limit 3
check "cache clear" "$BIN" --cache-clear

echo
echo "output formats"
check "json output" "$BIN" -o json account info
check "yaml output" "$BIN" -o yaml token list --limit 3

echo
echo "error handling"
check_fail "missing credentials rejected" env SUBROUTER_CREDENTIALS=/nonexistent/creds.json "$BIN" account info
check_fail "unknown token id rejected" "$BIN" token show 999999999
check_fail "bad flag rejected" "$BIN" token create --type nonsense --name x

echo
echo "───────────────────────────────────────────────────────────────────────────"
printf 'passed \033[32m%d\033[0m, failed \033[31m%d\033[0m\n\n' "$PASS" "$FAIL"
(( FAIL == 0 ))
