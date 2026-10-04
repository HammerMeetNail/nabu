#!/usr/bin/env bash
# Shared by the scheduled legacy verifier and its operations regression.
# The labels, rather than a name prefix, define which resources this job owns.

VERIFY_LABEL_PREFIX=io.hammermeetnail.backup-verifier

verify_label() {
    printf '%s.%s' "$VERIFY_LABEL_PREFIX" "$1"
}

verify_owned() {
    local kind="$1" resource="$2" run="$3" actual_service actual_run actual_kind
    if [[ "$kind" == container ]]; then
        actual_service=$(podman container inspect --format "{{index .Config.Labels \"$(verify_label service)\"}}" "$resource" 2>/dev/null) || return 1
        actual_run=$(podman container inspect --format "{{index .Config.Labels \"$(verify_label run)\"}}" "$resource" 2>/dev/null) || return 1
        actual_kind=$(podman container inspect --format "{{index .Config.Labels \"$(verify_label kind)\"}}" "$resource" 2>/dev/null) || return 1
    else
        actual_service=$(podman volume inspect --format "{{index .Labels \"$(verify_label service)\"}}" "$resource" 2>/dev/null) || return 1
        actual_run=$(podman volume inspect --format "{{index .Labels \"$(verify_label run)\"}}" "$resource" 2>/dev/null) || return 1
        actual_kind=$(podman volume inspect --format "{{index .Labels \"$(verify_label kind)\"}}" "$resource" 2>/dev/null) || return 1
    fi
    [[ "$actual_service" == "$VERIFY_SERVICE" && "$actual_run" == "$run" && "$actual_kind" == "$kind" ]]
}

verify_reconcile() {
    # The per-service lock is held here: no previous run of this verifier is
    # active, even when its PostgreSQL container survived a host crash.
    local ids id run volumes volume active failed=0
    ids=$(podman ps -a --filter "label=$(verify_label service)=$VERIFY_SERVICE" \
        --filter "label=$(verify_label kind)=container" --format '{{.ID}}') || return 1
    while IFS= read -r id; do
        [[ -n "$id" ]] || continue
        run=$(podman container inspect --format "{{index .Config.Labels \"$(verify_label run)\"}}" "$id") || return 1
        verify_owned container "$id" "$run" || { printf 'Verifier reconciliation rejected container %s\n' "$id" >&2; return 1; }
        if podman rm -f "$id" >/dev/null; then
            printf 'Verifier reconciliation removed container %s\n' "$id"
        else
            printf 'Verifier reconciliation failed container %s\n' "$id" >&2
            failed=1
        fi
    done <<< "$ids"

    volumes=$(podman volume ls --filter "label=$(verify_label service)=$VERIFY_SERVICE" \
        --filter "label=$(verify_label kind)=volume" --format '{{.Name}}') || return 1
    while IFS= read -r volume; do
        [[ -n "$volume" ]] || continue
        run=$(podman volume inspect --format "{{index .Labels \"$(verify_label run)\"}}" "$volume") || return 1
        verify_owned volume "$volume" "$run" || { printf 'Verifier reconciliation rejected volume %s\n' "$volume" >&2; return 1; }
        active=$(podman ps --filter "label=$(verify_label service)=$VERIFY_SERVICE" \
            --filter "label=$(verify_label run)=$run" --format '{{.ID}}') || return 1
        if [[ -n "$active" ]]; then
            printf 'Verifier reconciliation retained active volume %s\n' "$volume"
            continue
        fi
        if podman volume rm "$volume" >/dev/null; then
            printf 'Verifier reconciliation removed volume %s\n' "$volume"
        else
            printf 'Verifier reconciliation failed volume %s\n' "$volume" >&2
            failed=1
        fi
    done <<< "$volumes"
    return "$failed"
}

verify_cleanup() {
    local failed=0 exists_status
    if [[ -n "${VERIFY_CONTAINER:-}" ]] && podman container exists "$VERIFY_CONTAINER"; then
        if ! verify_owned container "$VERIFY_CONTAINER" "$VERIFY_RUN_ID"; then
            printf 'Verifier cleanup rejected container %s\n' "$VERIFY_CONTAINER" >&2
            failed=1
        elif podman rm -f "$VERIFY_CONTAINER" >/dev/null; then
            printf 'Verifier cleanup removed container %s\n' "$VERIFY_CONTAINER"
        else
            printf 'Verifier cleanup failed container %s\n' "$VERIFY_CONTAINER" >&2
            failed=1
        fi
    else
        exists_status=$?
        if [[ -n "${VERIFY_CONTAINER:-}" && "$exists_status" -ne 1 ]]; then
            printf 'Verifier cleanup could not inspect container %s\n' "$VERIFY_CONTAINER" >&2
            failed=1
        fi
    fi
    if [[ -n "${VERIFY_VOLUME:-}" ]] && podman volume exists "$VERIFY_VOLUME"; then
        if ! verify_owned volume "$VERIFY_VOLUME" "$VERIFY_RUN_ID"; then
            printf 'Verifier cleanup rejected volume %s\n' "$VERIFY_VOLUME" >&2
            failed=1
        elif podman volume rm "$VERIFY_VOLUME" >/dev/null; then
            printf 'Verifier cleanup removed volume %s\n' "$VERIFY_VOLUME"
        else
            printf 'Verifier cleanup failed volume %s\n' "$VERIFY_VOLUME" >&2
            failed=1
        fi
    else
        exists_status=$?
        if [[ -n "${VERIFY_VOLUME:-}" && "$exists_status" -ne 1 ]]; then
            printf 'Verifier cleanup could not inspect volume %s\n' "$VERIFY_VOLUME" >&2
            failed=1
        fi
    fi
    if [[ -n "${VERIFY_WORK:-}" && -d "$VERIFY_WORK" ]]; then
        if ! rm -rf -- "$VERIFY_WORK"; then
            printf 'Verifier cleanup failed scratch directory\n' >&2
            failed=1
        fi
    fi
    return "$failed"
}

verify_exit() {
    local status="$1"
    trap - EXIT HUP INT TERM
    if ! verify_cleanup; then status=1; fi
    if [[ "$status" -ne 0 ]]; then
        printf 'Backup verification failed (status %s)\n' "$status" >&2
    fi
    exit "$status"
}

verify_setup() {
    VERIFY_SERVICE="$1"
    VERIFY_CONTAINER=""
    VERIFY_VOLUME=""
    VERIFY_WORK=""
    VERIFY_RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')"
    umask 077
    trap 'verify_exit $?' EXIT
    trap 'exit 129' HUP
    trap 'exit 130' INT
    trap 'exit 143' TERM

    local state_dir="${VERIFY_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/backup-verifier}"
    mkdir -p "$state_dir" || return 1
    exec 9>"$state_dir/$VERIFY_SERVICE.lock" || return 1
    flock -w 3600 9 || { printf 'Verifier lock unavailable\n' >&2; return 1; }
    verify_reconcile || { printf 'Verifier reconciliation incomplete\n' >&2; return 1; }

    VERIFY_WORK=$(mktemp -d "${TMPDIR:-/tmp}/$VERIFY_SERVICE-backup-verify.XXXXXXXX") || return 1
    VERIFY_CONTAINER="$VERIFY_SERVICE-backup-verify-$VERIFY_RUN_ID"
    VERIFY_VOLUME="$VERIFY_CONTAINER-data"
    podman volume create \
        --label "$(verify_label service)=$VERIFY_SERVICE" \
        --label "$(verify_label run)=$VERIFY_RUN_ID" \
        --label "$(verify_label kind)=volume" \
        "$VERIFY_VOLUME" >/dev/null || return 1
    verify_owned volume "$VERIFY_VOLUME" "$VERIFY_RUN_ID" || {
        printf 'Verifier volume ownership mismatch: %s\n' "$VERIFY_VOLUME" >&2
        return 1
    }
}

verify_run_postgres() {
    podman run -d \
        --name "$VERIFY_CONTAINER" \
        --label "$(verify_label service)=$VERIFY_SERVICE" \
        --label "$(verify_label run)=$VERIFY_RUN_ID" \
        --label "$(verify_label kind)=container" \
        --mount "type=volume,source=$VERIFY_VOLUME,destination=/var/lib/postgresql/data" \
        -e "POSTGRES_USER=$VERIFY_DB_USER" \
        -e "POSTGRES_PASSWORD=$TEST_DB_PASSWORD" \
        -e "POSTGRES_DB=$TEST_DB_NAME" \
        docker.io/library/postgres:16-alpine postgres \
        -c log_min_messages=panic -c log_min_error_statement=panic \
        -c log_error_verbosity=terse -c log_statement=none \
        -c log_parameter_max_length=0 -c log_parameter_max_length_on_error=0
}
