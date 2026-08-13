#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -Eeuo pipefail
set +x
umask 077

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd -P)
CDPATH= cd -- "$REPO_ROOT" || {
  printf '[publish-dev] error: could not enter repository root\n' >&2
  exit 1
}
ATLAS_ENV_FILE=${ATLAS_ENV_FILE:-${HOME:?HOME is required}/.config/coze-studio/dev-atlas.env}
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
GIT_BIN=${GIT_BIN:-git}
DOCKER_BIN=${DOCKER_BIN:-docker}
MIGRATION_POLICY_SCRIPT=$REPO_ROOT/scripts/database/check-migration-policy.sh
SCHEMA_DRIFT_SCRIPT=$REPO_ROOT/deploy/dev/check-schema-drift.sh
ATLAS_STATUS_FORMAT='COZE_ATLAS_STATUS|{{ .Status }}|{{ .Current }}|{{ .Count }}{{ "\n" }}'
atlas_env_file=''
atlas_url=''
atlas_userinfo=''
atlas_password=''
atlas_output_file=''
atlas_snapshot_dir=''
atlas_runtime_env=''
publish_tmp_root=''
publish_snapshot_prefix=''
publish_output_prefix=''
atlas_current_version=''
atlas_pending_count=''
atlas_target_version=''

is_revision() {
  [[ "${1:-}" =~ ^[0-9a-fA-F]{40}$ ]]
}

normalize_revision() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

git_cmd() {
  "$GIT_BIN" "$@"
}

docker_cmd() {
  "$DOCKER_BIN" "$@"
}

error() {
  printf '[publish-dev] error: %s\n' "$*" >&2
}

log() {
  printf '[publish-dev] %s\n' "$*"
}

die() {
  error "$*"
  exit 1
}

file_mode() {
  local mode

  if mode=$(stat -c '%a' "$1" 2>/dev/null) && [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  if mode=$(stat -f '%Lp' "$1" 2>/dev/null) && [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  return 1
}

physical_parent_is_publish_tmp_root() {
  local path=$1
  local parent
  local physical_parent

  [ -n "$publish_tmp_root" ] || return 1
  parent=$(dirname -- "$path") || return 1
  physical_parent=$(CDPATH= cd -- "$parent" 2>/dev/null && pwd -P) || return 1
  [ "$physical_parent" = "$publish_tmp_root" ]
}

is_controlled_atlas_output_file() {
  local path=$1

  [ -n "$publish_output_prefix" ] || return 1
  case "$path" in
    "$publish_output_prefix".*) ;;
    *) return 1 ;;
  esac
  [ "$path" != / ] && [ -f "$path" ] && [ ! -L "$path" ] && \
    physical_parent_is_publish_tmp_root "$path"
}

is_controlled_atlas_snapshot_dir() {
  local path=$1

  [ -n "$publish_snapshot_prefix" ] || return 1
  case "$path" in
    "$publish_snapshot_prefix".*) ;;
    *) return 1 ;;
  esac
  [ "$path" != / ] && [ -d "$path" ] && [ ! -L "$path" ] && \
    physical_parent_is_publish_tmp_root "$path"
}

is_controlled_atlas_runtime_env() {
  local path=$1
  local parent
  local physical_parent
  local physical_snapshot

  is_controlled_atlas_snapshot_dir "$atlas_snapshot_dir" || return 1
  [ "$path" = "$atlas_snapshot_dir/atlas-runtime.env" ] || return 1
  [ -f "$path" ] && [ ! -L "$path" ] || return 1
  parent=$(dirname -- "$path") || return 1
  physical_parent=$(CDPATH= cd -- "$parent" 2>/dev/null && pwd -P) || return 1
  physical_snapshot=$(CDPATH= cd -- "$atlas_snapshot_dir" 2>/dev/null && pwd -P) || return 1
  [ "$physical_parent" = "$physical_snapshot" ]
}

cleanup_atlas_output() {
  if [ -n "${atlas_output_file:-}" ]; then
    if is_controlled_atlas_output_file "$atlas_output_file"; then
      rm -f -- "$atlas_output_file" >/dev/null 2>&1 || true
    fi
    atlas_output_file=''
  fi
}

cleanup_publish_artifacts() {
  cleanup_atlas_output
  if [ -n "${atlas_snapshot_dir:-}" ]; then
    if is_controlled_atlas_snapshot_dir "$atlas_snapshot_dir"; then
      rm -rf -- "$atlas_snapshot_dir" >/dev/null 2>&1 || true
    fi
    atlas_snapshot_dir=''
    atlas_runtime_env=''
  fi
}

replace_literal() {
  local source=$1
  local needle=$2
  local replacement=$3
  local prefix
  local result=''

  if [ -z "$needle" ]; then
    redacted_text=$source
    return 0
  fi
  while [[ "$source" == *"$needle"* ]]; do
    prefix=${source%%"$needle"*}
    result=$result$prefix$replacement
    source=${source#*"$needle"}
  done
  redacted_text=$result$source
}

redact_atlas_line() {
  local line=$1
  local match
  local field_prefix
  local field_key
  local restore_nocasematch=0

  replace_literal "$line" "$atlas_url" '[REDACTED_ATLAS_URL]'
  line=$redacted_text
  replace_literal "$line" "$atlas_userinfo" '[REDACTED_USERINFO]'
  line=$redacted_text
  replace_literal "$line" "$atlas_password" '[REDACTED_PASSWORD]'
  line=$redacted_text

  while [[ "$line" =~ mysql://[^[:space:]]+ ]]; do
    match=${BASH_REMATCH[0]}
    replace_literal "$line" "$match" '[REDACTED_MYSQL_URI]'
    line=$redacted_text
  done
  while [[ "$line" =~ [[:alpha:]][[:alnum:].+-]*://[^/@[:space:]]+@ ]]; do
    match=${BASH_REMATCH[0]}
    replace_literal "$line" "$match" '[REDACTED_URL_AUTHORITY]'
    line=$redacted_text
  done
  while [[ "$line" =~ [^[:space:]/@:]+:[^[:space:]@]+@[^[:space:]]+ ]]; do
    match=${BASH_REMATCH[0]}
    replace_literal "$line" "$match" '[REDACTED_USERINFO_TOKEN]'
    line=$redacted_text
  done
  if ! shopt -q nocasematch; then
    shopt -s nocasematch
    restore_nocasematch=1
  fi
  while [[ "$line" =~ (^|[^[:alnum:]_.-])((api|access|secret)[[:space:]]+key|client[[:space:]]+secret|api[[:space:]]+token)[[:space:]]*[:=][[:space:]]*[^[:space:]]+ ]]; do
    match=${BASH_REMATCH[0]}
    field_prefix=${BASH_REMATCH[1]}
    field_key=${BASH_REMATCH[2]}
    replace_literal "$line" "$match" \
      "${field_prefix}${field_key} [REDACTED_SECRET_FIELD]"
    line=$redacted_text
  done
  while [[ "$line" =~ (^|[^[:alnum:]_.-])([[:alnum:]_.-]*(password|passwd|pwd|secret|token|api[_-]key|apikey|access[_-]key|accesskey|secret[_-]key|secretkey))[[:space:]]*[:=][[:space:]]*[^[:space:]]+ ]]; do
    match=${BASH_REMATCH[0]}
    field_prefix=${BASH_REMATCH[1]}
    field_key=${BASH_REMATCH[2]}
    replace_literal "$line" "$match" \
      "${field_prefix}${field_key} [REDACTED_SECRET_FIELD]"
    line=$redacted_text
  done
  if [ "$restore_nocasematch" -eq 1 ]; then
    shopt -u nocasematch
  fi

  printf '%s\n' "$line"
}

emit_redacted_atlas_output() {
  local line

  while IFS= read -r line || [ -n "$line" ]; do
    redact_atlas_line "$line"
  done <"$atlas_output_file"
}

create_atlas_output_file() {
  local mode

  atlas_output_file=$(mktemp "$publish_output_prefix.XXXXXX") || \
    die 'could not create a secure Atlas output file'
  is_controlled_atlas_output_file "$atlas_output_file" || \
    die 'Atlas output file path failed safety validation'
  chmod 600 "$atlas_output_file" || die 'could not secure the Atlas output file'
  mode=$(file_mode "$atlas_output_file") || die 'Atlas output file mode could not be read'
  [ "$mode" = 600 ] || die 'Atlas output file mode must be exactly 600'
}

remove_atlas_output_file() {
  is_controlled_atlas_output_file "$atlas_output_file" || \
    die 'Atlas output file path failed safety validation'
  rm -f -- "$atlas_output_file" || die 'could not remove the Atlas output file'
  atlas_output_file=''
}

run_atlas_step() {
  local stage=$1
  local status
  shift

  create_atlas_output_file
  if docker_cmd "$@" >"$atlas_output_file" 2>&1; then
    status=0
  else
    status=$?
  fi
  emit_redacted_atlas_output
  remove_atlas_output_file

  [ "$status" -eq 0 ] || die "Atlas migration $stage failed"
  log "Atlas migration $stage passed"
}

validate_env_file() {
  local requested_path=$1
  local env_dir
  local env_name
  local physical_dir
  local mode
  local line
  local atlas_url_count=0
  local authority

  atlas_url=''
  atlas_userinfo=''
  atlas_password=''

  [ ! -L "$requested_path" ] || die 'Atlas env file must not be a symbolic link'
  [ -f "$requested_path" ] || die 'Atlas env file must exist as a regular file'

  env_dir=$(dirname -- "$requested_path")
  env_name=$(basename -- "$requested_path")
  physical_dir=$(CDPATH= cd -- "$env_dir" 2>/dev/null && pwd -P) || \
    die 'Atlas env file path could not be resolved'
  atlas_env_file=$physical_dir/$env_name

  [ ! -L "$atlas_env_file" ] || die 'Atlas env file must not be a symbolic link'
  [ -f "$atlas_env_file" ] || die 'Atlas env file must exist as a regular file'
  case "$atlas_env_file" in
    "$REPO_ROOT" | "$REPO_ROOT"/*)
      die 'Atlas env file must be outside the repository'
      ;;
  esac

  mode=$(file_mode "$atlas_env_file") || die 'Atlas env file mode could not be read'
  [ "$mode" = 600 ] || die 'Atlas env file mode must be exactly 600'

  while IFS= read -r line || [ -n "$line" ]; do
    if [[ "$line" =~ ^[[:space:]]*$ ]] || [[ "$line" =~ ^[[:space:]]*# ]]; then
      continue
    fi
    case "$line" in
      ATLAS_URL=*)
        atlas_url_count=$((atlas_url_count + 1))
        [ "$atlas_url_count" -eq 1 ] || \
          die 'Atlas env file must contain exactly one ATLAS_URL entry'
        atlas_url=${line#ATLAS_URL=}
        [ -n "$atlas_url" ] || die 'ATLAS_URL must not be empty'
        [[ "$atlas_url" == mysql://?* ]] || die 'ATLAS_URL must use the mysql scheme'
        ;;
      *)
        die 'Atlas env file contains an unsupported entry'
        ;;
    esac
  done <"$atlas_env_file"

  [ "$atlas_url_count" -eq 1 ] || \
    die 'Atlas env file must contain exactly one ATLAS_URL entry'

  authority=${atlas_url#mysql://}
  case "$authority" in
    *@*)
      atlas_userinfo=${authority%%@*}
      case "$atlas_userinfo" in
        *:*) atlas_password=${atlas_userinfo#*:} ;;
      esac
      ;;
  esac
}

validate_publish_tmp_root() {
  local requested_tmp_root=${TMPDIR:-/tmp}
  local physical_tmp_root

  [ -d "$requested_tmp_root" ] || die 'publish temp root must be an existing directory'
  physical_tmp_root=$(CDPATH= cd -- "$requested_tmp_root" 2>/dev/null && pwd -P) || \
    die 'publish temp root could not be physically resolved'
  case "$physical_tmp_root" in
    "$REPO_ROOT" | "$REPO_ROOT"/*)
      die 'publish temp root must be outside the repository'
      ;;
  esac

  publish_tmp_root=$physical_tmp_root
  if [ "$publish_tmp_root" = / ]; then
    publish_snapshot_prefix=/coze-publish-dev-snapshot
    publish_output_prefix=/coze-publish-dev-atlas
  else
    publish_snapshot_prefix=$publish_tmp_root/coze-publish-dev-snapshot
    publish_output_prefix=$publish_tmp_root/coze-publish-dev-atlas
  fi
}

create_atlas_snapshot() {
  local target_sha=$1
  local mode

  atlas_snapshot_dir=$(mktemp -d "$publish_snapshot_prefix.XXXXXX") || \
    die 'could not create Atlas snapshot directory'
  is_controlled_atlas_snapshot_dir "$atlas_snapshot_dir" || \
    die 'Atlas snapshot directory path failed safety validation'
  chmod 700 "$atlas_snapshot_dir" || die 'could not secure Atlas snapshot directory'
  mode=$(file_mode "$atlas_snapshot_dir") || die 'Atlas snapshot directory mode could not be read'
  [ "$mode" = 700 ] || die 'Atlas snapshot directory mode must be exactly 700'

  if ! git_cmd archive --format=tar "$target_sha" -- \
    docker/atlas/migrations .github/atlas-dev.hcl | tar -x -C "$atlas_snapshot_dir"; then
    die 'failed to create Atlas snapshot'
  fi
  is_controlled_atlas_snapshot_dir "$atlas_snapshot_dir" || \
    die 'Atlas snapshot directory path failed safety validation'
  [ -d "$atlas_snapshot_dir/docker/atlas/migrations" ] && \
    [ ! -L "$atlas_snapshot_dir/docker/atlas/migrations" ] || \
    die 'Atlas snapshot migrations are missing or invalid'
  [ -f "$atlas_snapshot_dir/.github/atlas-dev.hcl" ] && \
    [ ! -L "$atlas_snapshot_dir/.github/atlas-dev.hcl" ] || \
    die 'Atlas snapshot config is missing or invalid'

  atlas_runtime_env=$atlas_snapshot_dir/atlas-runtime.env
  if ! (set -C; printf 'ATLAS_URL=%s\n' "$atlas_url" >"$atlas_runtime_env"); then
    die 'could not write Atlas runtime env file'
  fi
  is_controlled_atlas_runtime_env "$atlas_runtime_env" || \
    die 'Atlas runtime env file path failed safety validation'
  chmod 600 "$atlas_runtime_env" || die 'could not secure Atlas runtime env file'
  mode=$(file_mode "$atlas_runtime_env") || die 'Atlas runtime env file mode could not be read'
  [ "$mode" = 600 ] || die 'Atlas runtime env file mode must be exactly 600'
}

resolve_atlas_target_version() {
  local migration_files
  local migration_file
  local migration_name

  shopt -s nullglob
  migration_files=("$atlas_snapshot_dir"/docker/atlas/migrations/*.sql)
  shopt -u nullglob
  [ "${#migration_files[@]}" -gt 0 ] || die 'Atlas snapshot contains no migration SQL files'

  atlas_target_version=''
  for migration_file in "${migration_files[@]}"; do
    migration_name=${migration_file##*/}
    [[ "$migration_name" =~ ^([0-9]{14})_.*\.sql$ ]] || \
      die "Atlas snapshot contains an invalid migration filename: $migration_name"
    atlas_target_version=${BASH_REMATCH[1]}
  done
  [[ "$atlas_target_version" =~ ^[0-9]{14}$ ]] || \
    die 'Atlas target migration version could not be resolved'
}

check_local_target() {
  local target_sha=$1
  local worktree_status
  local head_sha

  worktree_status=$(git_cmd status --porcelain --untracked-files=all) || \
    die 'failed to inspect worktree status'
  [ -z "$worktree_status" ] || die 'worktree must be clean'

  head_sha=$(git_cmd rev-parse HEAD) || die 'failed to read HEAD'
  [ "$(normalize_revision "$head_sha")" = "$target_sha" ] || \
    die 'HEAD does not match target dev SHA'
}

check_origin_and_history() {
  local expected_origin_sha=$1
  local target_sha=$2
  local remote_sha

  git_cmd fetch --no-tags origin dev >/dev/null 2>&1 || die 'failed to fetch origin/dev'
  remote_sha=$(git_cmd rev-parse FETCH_HEAD) || die 'failed to read fetched origin/dev'
  [ "$(normalize_revision "$remote_sha")" = "$expected_origin_sha" ] || \
    die 'fetched origin/dev does not match expected SHA'

  git_cmd cat-file -e "${target_sha}^{commit}" >/dev/null 2>&1 || \
    die 'target commit does not exist'
  git_cmd merge-base --is-ancestor "$expected_origin_sha" "$target_sha" >/dev/null 2>&1 || \
    die 'expected origin/dev is not an ancestor of target'
}

check_migration_policy() {
  local expected_origin_sha=$1
  local target_sha=$2

  [ -x "$MIGRATION_POLICY_SCRIPT" ] || die 'migration policy checker is missing or not executable'
  "$MIGRATION_POLICY_SCRIPT" "$expected_origin_sha" "$target_sha" || \
    die 'migration policy check failed'
}

run_atlas_status() {
  local status
  local marker_count=0
  local line
  local marker
  local parsed_status
  local parsed_current
  local parsed_count

  run_atlas_step validation run --rm \
    -v "$atlas_snapshot_dir/docker/atlas/migrations:/migrations:ro" \
    "$ATLAS_IMAGE" \
    migrate validate --dir file:///migrations

  create_atlas_output_file
  if docker_cmd run --rm \
    --env-file "$atlas_runtime_env" \
    -v "$atlas_snapshot_dir/docker/atlas/migrations:/migrations:ro" \
    -v "$atlas_snapshot_dir/.github/atlas-dev.hcl:/atlas.hcl:ro" \
    "$ATLAS_IMAGE" \
    migrate status --config file:///atlas.hcl --env dev \
      --format "$ATLAS_STATUS_FORMAT" >"$atlas_output_file" 2>&1; then
    status=0
  else
    status=$?
  fi

  atlas_current_version=''
  atlas_pending_count=''
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      COZE_ATLAS_STATUS\|*)
        marker_count=$((marker_count + 1))
        marker=$line
        IFS='|' read -r _ parsed_status parsed_current parsed_count <<<"$marker"
        ;;
    esac
  done <"$atlas_output_file"

  emit_redacted_atlas_output
  remove_atlas_output_file

  [ "$status" -eq 0 ] || die 'Atlas migration status failed'
  [ "$marker_count" -eq 1 ] || die 'Atlas migration status output was not uniquely parseable'
  [[ "$parsed_status" =~ ^[A-Z_]+$ ]] || die 'Atlas migration status value is invalid'
  [[ "$parsed_current" =~ ^[0-9]{14}$ ]] || \
    die 'Atlas current migration version is missing or invalid'
  [[ "$parsed_count" =~ ^[0-9]+$ ]] || die 'Atlas pending migration count is invalid'
  atlas_current_version=$parsed_current
  atlas_pending_count=$parsed_count
  log "Atlas migration status passed (current=$atlas_current_version pending=$atlas_pending_count)"
}

run_atlas_apply() {
  run_atlas_step apply run --rm \
    --env-file "$atlas_runtime_env" \
    -v "$atlas_snapshot_dir/docker/atlas/migrations:/migrations:ro" \
    -v "$atlas_snapshot_dir/.github/atlas-dev.hcl:/atlas.hcl:ro" \
    "$ATLAS_IMAGE" \
    migrate apply --config file:///atlas.hcl --env dev
}

run_schema_drift_check() {
  local stage=$1
  local expected_version=$2

  [ -x "$SCHEMA_DRIFT_SCRIPT" ] || die 'schema drift checker is missing or not executable'
  "$SCHEMA_DRIFT_SCRIPT" \
    --migrations-dir "$atlas_snapshot_dir/docker/atlas/migrations" \
    --atlas-env-file "$atlas_runtime_env" \
    --version "$expected_version" \
    --stage "$stage" || die "schema drift check failed during $stage"
}

main() {
  local expected_origin_sha
  local target_sha
  local current_branch
  local publish_mode=release

  if [ "${1:-}" = --status ]; then
    [ "$#" -eq 3 ] || \
      die 'status mode expects: --status <expected-origin-dev-sha> <target-dev-sha>'
    publish_mode=status
    shift
  else
    [ "$#" -eq 2 ] || \
      die 'expected exactly two arguments: <expected-origin-dev-sha> <target-dev-sha>'
  fi
  is_revision "$1" || \
    die 'expected origin/dev SHA must be exactly 40 hexadecimal characters'
  is_revision "$2" || die 'target dev SHA must be exactly 40 hexadecimal characters'
  expected_origin_sha=$(normalize_revision "$1")
  target_sha=$(normalize_revision "$2")

  current_branch=$(git_cmd symbolic-ref --quiet --short HEAD) || \
    die 'current branch must be dev'
  [ "$current_branch" = dev ] || die 'current branch must be dev'

  check_local_target "$target_sha"
  check_origin_and_history "$expected_origin_sha" "$target_sha"
  check_migration_policy "$expected_origin_sha" "$target_sha"
  validate_env_file "$ATLAS_ENV_FILE"
  validate_publish_tmp_root
  create_atlas_snapshot "$target_sha"
  resolve_atlas_target_version
  run_atlas_status
  run_schema_drift_check pre-apply "$atlas_current_version"
  if [ "$publish_mode" = status ]; then
    check_local_target "$target_sha"
    check_origin_and_history "$expected_origin_sha" "$target_sha"
    log "inspected Atlas status for audited dev revision $target_sha; no migration was applied and no push was attempted"
    return 0
  fi
  run_atlas_apply
  run_schema_drift_check post-apply "$atlas_target_version"
  check_local_target "$target_sha"
  check_origin_and_history "$expected_origin_sha" "$target_sha"

  if ! git_cmd push origin "$target_sha:refs/heads/dev" >/dev/null 2>&1; then
    die 'dev push failed'
  fi
  log "pushed audited dev revision $target_sha; GitHub and Baota own all remaining steps"
}

trap cleanup_publish_artifacts EXIT
trap 'exit 1' HUP INT TERM

main "$@"
