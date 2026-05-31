#!/bin/bash
# Resend email notifications for ops scripts.
#
# Env:
#   RESEND_API_KEY (required)
#   EMAIL_FROM_ADDRESS (required, e.g. "Nabu <noreply@yearofbingo.com>" or "noreply@yearofbingo.com")
#   BACKUP_NOTIFY_EMAILS (required, comma-separated recipients)
#   BACKUP_NOTIFY_SUCCESS (optional, default: 1) - send success emails for backup.sh
#   BACKUP_NOTIFY_VERIFY_SUCCESS (optional, default: 0) - send success emails for verify-backup.sh

set -euo pipefail

json_escape() {
    local s="${1:-}"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    s="${s//$'\r'/\\r}"
    s="${s//$'\t'/\\t}"
    printf '%s' "$s"
}

to_json_array() {
    local raw="${1:-}"
    local IFS=,
    local -a parts=()
    read -ra parts <<< "$raw"

    local out="["
    local first=1
    local part
    for part in "${parts[@]}"; do
        part="${part#"${part%%[![:space:]]*}"}"
        part="${part%"${part##*[![:space:]]}"}"
        [[ -z "$part" ]] && continue

        if [[ $first -eq 0 ]]; then
            out+=","
        fi
        out+="\"$(json_escape "$part")\""
        first=0
    done
    out+="]"
    printf '%s' "$out"
}

notify_email() {
    local subject="${1:-}"
    local body="${2:-}"

    if [[ -z "${BACKUP_NOTIFY_EMAILS:-}" ]]; then
        return 0
    fi

    if [[ -z "${RESEND_API_KEY:-}" ]] || [[ -z "${EMAIL_FROM_ADDRESS:-}" ]]; then
        echo "WARN: email notification skipped (missing RESEND_API_KEY or EMAIL_FROM_ADDRESS)" >&2
        return 0
    fi

    if ! command -v curl &>/dev/null; then
        echo "WARN: email notification skipped (curl not installed)" >&2
        return 0
    fi

    local to_json
    to_json="$(to_json_array "$BACKUP_NOTIFY_EMAILS")"
    if [[ "$to_json" == "[]" ]]; then
        echo "WARN: email notification skipped (BACKUP_NOTIFY_EMAILS empty after parsing)" >&2
        return 0
    fi

    local payload
    payload="{\"from\":\"$(json_escape "$EMAIL_FROM_ADDRESS")\",\"to\":${to_json},\"subject\":\"$(json_escape "$subject")\",\"text\":\"$(json_escape "$body")\"}"

    curl -fsS https://api.resend.com/emails \
        -H "Authorization: Bearer ${RESEND_API_KEY}" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        >/dev/null
}
