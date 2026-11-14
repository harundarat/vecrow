#!/usr/bin/env bash
# Funds a Vecrow escrow on testnet with the mock USDC token (mock-ft).
#
# 1Click has no testnet, so on testnet the client's "pay from any chain" step
# is replaced by a direct ft_transfer_call from a NEAR testnet account. The
# contract accepts funds from any sender: it checks the token, that the escrow
# is Created and that the amount covers the total.
#
# Usage: scripts/fund-escrow-testnet.sh <escrow-id> <payer-account>
#
#   <escrow-id>      the ID in the escrow page URL, /escrows/<escrow-id>
#   <payer-account>  a testnet account whose key the near CLI can sign with
#
# Environment (read from backend/.env when not set):
#   ESCROW_CONTRACT_ID  the escrow contract account
#   USDC_CONTRACT_ID    the mock-ft account the escrow contract was deployed with
#   NEAR_RPC_URL        a testnet RPC endpoint
# Optional:
#   NEAR_SIGN_WITH      near CLI signing method (default sign-with-keychain;
#                       sign-with-legacy-keychain reads ~/.near-credentials)
#   ENV_FILE            another file to read the variables above from
#
# Needs curl, jq and near (near-cli-rs: cargo install near-cli-rs --locked).
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }

[ $# -eq 2 ] || { echo "usage: $0 <escrow-id> <payer-account>" >&2; exit 2; }
escrow_id=$1
payer=$2

for cmd in curl jq near; do
    command -v "$cmd" >/dev/null || die "$cmd is not installed"
done

env_file=${ENV_FILE:-"$(cd "$(dirname "$0")/../.." && pwd)/backend/.env"}
from_env_file() { # prints KEY's value from env_file, without surrounding quotes
    [ -f "$env_file" ] || return 0
    sed -n "s/^$1=//p" "$env_file" | tail -n 1 | sed 's/^["'\'']//; s/["'\'']$//'
}
escrow_contract=${ESCROW_CONTRACT_ID:-$(from_env_file ESCROW_CONTRACT_ID)}
usdc=${USDC_CONTRACT_ID:-$(from_env_file USDC_CONTRACT_ID)}
rpc=${NEAR_RPC_URL:-$(from_env_file NEAR_RPC_URL)}
sign_with=${NEAR_SIGN_WITH:-sign-with-keychain}
[ -n "$escrow_contract" ] || die "set ESCROW_CONTRACT_ID (or fill it in $env_file)"
[ -n "$usdc" ] || die "set USDC_CONTRACT_ID (or fill it in $env_file)"
[ -n "$rpc" ] || die "set NEAR_RPC_URL (or fill it in $env_file)"

rpc_call() { # rpc_call <json-rpc body>
    curl -sS --fail --max-time 30 "$rpc" -H 'Content-Type: application/json' -d "$1"
}

# mock-ft has an open mint, so its tokens are worthless by design. Refuse any
# network but testnet: on mainnet, escrows are funded through 1Click.
chain=$(rpc_call '{"jsonrpc":"2.0","id":"vecrow","method":"status","params":[]}' | jq -r '.result.chain_id')
[ "$chain" = testnet ] || die "NEAR_RPC_URL ($rpc) is on '$chain', not testnet"

view() { # view <contract> <method> <json args>: prints the JSON result
    local body
    body=$(jq -nc --arg c "$1" --arg m "$2" --arg a "$(printf '%s' "$3" | base64 | tr -d '\n')" \
        '{jsonrpc: "2.0", id: "vecrow", method: "query",
          params: {request_type: "call_function", finality: "final",
                   account_id: $c, method_name: $m, args_base64: $a}}')
    rpc_call "$body" | jq -er --arg where "$1.$2" '
        if .error then "\($where): \(.error.cause.name // .error.message)\n" | halt_error(1)
        elif .result.error then "\($where): \(.result.error)\n" | halt_error(1)
        else .result.result | implode end'
}

call() { # call <contract> <method> <json args> <deposit> <gas in Tgas>
    near contract call-function as-transaction "$1" "$2" json-args "$3" \
        prepaid-gas "$5.0 Tgas" attached-deposit "$4" \
        sign-as "$payer" network-config testnet "$sign_with" send
}

escrow=$(view "$escrow_contract" get_escrow "$(jq -nc --arg id "$escrow_id" '{escrow_id: $id}')")
[ "$escrow" != null ] || die "escrow $escrow_id is not on-chain: the creator approves it first (status Created)"
status=$(jq -r .status <<<"$escrow")
total=$(jq -r .total <<<"$escrow")
[ "$status" = Created ] || die "escrow $escrow_id is $status; only a Created escrow can be funded"
echo "escrow $escrow_id on $escrow_contract: $status, total $total (base units of $usdc)"

registered=$(view "$usdc" storage_balance_of "$(jq -nc --arg a "$payer" '{account_id: $a}')")
if [ "$registered" = null ]; then
    min=$(view "$usdc" storage_balance_bounds '{}' | jq -r .min)
    echo "registering $payer on $usdc ($min yoctoNEAR)"
    call "$usdc" storage_deposit "$(jq -nc --arg a "$payer" '{account_id: $a, registration_only: true}')" \
        "$min yoctoNEAR" 30
fi

echo "minting $total test USDC to $payer"
call "$usdc" mint "$(jq -nc --arg a "$payer" --arg n "$total" '{account_id: $a, amount: $n}')" "0 NEAR" 30

echo "sending $total to $escrow_contract with msg $escrow_id"
call "$usdc" ft_transfer_call \
    "$(jq -nc --arg r "$escrow_contract" --arg n "$total" --arg m "$escrow_id" '{receiver_id: $r, amount: $n, msg: $m}')" \
    "1 yoctoNEAR" 100

# Views read final blocks, which trail the transaction by a few seconds.
for _ in 1 2 3 4 5 6; do
    status=$(view "$escrow_contract" get_escrow "$(jq -nc --arg id "$escrow_id" '{escrow_id: $id}')" | jq -r .status)
    [ "$status" = Funded ] && break
    sleep 2
done
[ "$status" = Funded ] || die "escrow $escrow_id is still $status: the contract returned the tokens (see the receipt logs above)"
echo "escrow $escrow_id is Funded; reload the escrow page to see it"
