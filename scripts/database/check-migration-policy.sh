#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

usage() {
  printf 'usage: %s <base-sha> <target-sha> [--allow-special]\n' "${0##*/}" >&2
  exit 2
}

fail() {
  printf 'migration policy failure: %s\n' "$*" >&2
  exit 1
}

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage

base_sha=$1
target_sha=$2
allow_special=false

if [ "$#" -eq 3 ]; then
  [ "$3" = '--allow-special' ] || usage
  allow_special=true
fi

git cat-file -e "${base_sha}^{commit}" 2>/dev/null || fail 'base SHA is not a commit'
git cat-file -e "${target_sha}^{commit}" 2>/dev/null || fail 'target SHA is not a commit'

changed_migrations=$(git diff --name-status --no-renames "$base_sha" "$target_sha" -- \
  'docker/atlas/migrations/*.sql')

[ -n "$changed_migrations" ] || {
  printf '%s\n' 'migration policy passed: no migration SQL changes'
  exit 0
}

added_count=0
base_max_version=''

while IFS= read -r base_path; do
  base_filename=${base_path##*/}
  if [[ "$base_filename" =~ ^([0-9]{14})_.*\.sql$ ]]; then
    base_version=${BASH_REMATCH[1]}
    if [ -z "$base_max_version" ] || [[ "$base_version" > "$base_max_version" ]]; then
      base_max_version=$base_version
    fi
  fi
done < <(git ls-tree -r --name-only "$base_sha" -- docker/atlas/migrations)

while IFS=$'\t' read -r status path extra_path; do
  [ -n "$status" ] || continue
  [ -z "${extra_path:-}" ] || fail "unexpected migration path tuple for $path"

  case "$status" in
    A)
      ;;
    *)
      fail "existing migration files are immutable: $path ($status)"
      ;;
  esac

  filename=${path##*/}
  if [[ ! "$filename" =~ ^[0-9]{14}_(expand|data|contract|repair)_[a-z0-9]+(_[a-z0-9]+)+\.sql$ ]]; then
    fail "invalid migration filename: $filename; expected YYYYMMDDHHMMSS_{expand|data|contract|repair}_module_action.sql"
  fi

  migration_type=${BASH_REMATCH[1]}
  migration_version=${filename%%_*}
  if [ -n "$base_max_version" ] && [[ "$migration_version" < "$base_max_version" || "$migration_version" = "$base_max_version" ]]; then
    fail "new migration version must be greater than the existing maximum $base_max_version: $filename"
  fi

  matching_version_count=0
  while IFS= read -r target_path; do
    target_filename=${target_path##*/}
    case "$target_filename" in
      "$migration_version"_*.sql)
        matching_version_count=$((matching_version_count + 1))
        ;;
    esac
  done < <(git ls-tree -r --name-only "$target_sha" -- docker/atlas/migrations)
  [ "$matching_version_count" -eq 1 ] || \
    fail "migration version must be unique: $migration_version"

  migration_sql=$(git show "$target_sha:$path") || fail "cannot read migration from target SHA: $path"

  case "$migration_type" in
    expand)
      if printf '%s\n' "$migration_sql" | grep -Eiq \
        '(^|[^[:alnum:]_])(DROP|TRUNCATE|RENAME|MODIFY)([^[:alnum:]_]|$)|CHANGE[[:space:]]+(COLUMN[[:space:]]+)?'; then
        fail "expand migration contains destructive SQL: $filename"
      fi
      ;;
    data)
      if printf '%s\n' "$migration_sql" | grep -Eiq \
        'DROP[[:space:]]+(TABLE|COLUMN)|TRUNCATE[[:space:]]+(TABLE[[:space:]]+)?'; then
        fail "data migration contains schema-destructive SQL: $filename"
      fi
      ;;
    contract|repair)
      if [ "$allow_special" != true ]; then
        fail "$migration_type migration requires a separate special migration approval: $filename"
      fi
      ;;
  esac

  added_count=$((added_count + 1))
done <<<"$changed_migrations"

printf 'migration policy passed: %s new migration(s) checked\n' "$added_count"
