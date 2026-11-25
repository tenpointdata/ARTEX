#!/usr/bin/env bash
# Check that db/migrations can land on a base branch without a version clash.
#
# Usage:
#   scripts/check-migration-versions.sh [BASE_REF]
#   scripts/check-migration-versions.sh --branches [BASE_REF] [BRANCH_GLOB]
#
# Default mode compares the working tree's db/migrations with BASE_REF
# (default: local main when it contains origin/main, else origin/main) and fails when:
#   - two files in the working tree share a version (db/migrate.go refuses it);
#   - a migration present where this branch forked from BASE_REF (the
#     merge-base) is missing, renamed or edited here (published migrations are
#     never changed); one that reached BASE_REF later is only noted;
#   - a migration added here has a version <= the newest on BASE_REF. The
#     runner (applyMigrations) refuses a pending migration older than the newest
#     applied one, so a new migration must be numbered above everything on base.
# Each failure names the next free number on BASE_REF.
#
# --branches is a read-only report: for every local branch matching
# BRANCH_GLOB (default ej/*) it lists the migrations the branch adds relative to
# BASE_REF, says which need a renumber, and flags versions that two branches
# both add. It exits 1 when anything needs action.
#
# Comparison is by file name and content: a branch file byte-identical to a base
# file (already landed) is not "added".
set -euo pipefail
cd "$(dirname "$0")/.."

DIR=db/migrations

usage() { awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"; }

# The newer of main and origin/main: local main when it already contains
# origin/main (agents merge into local main before pushing), else origin/main.
default_base() {
  if ! git rev-parse --verify --quiet origin/main >/dev/null; then echo main; return; fi
  if git rev-parse --verify --quiet main >/dev/null && git merge-base --is-ancestor origin/main main; then
    echo main
  else
    echo origin/main
  fi
}

# version_of FILE_NAME -> integer version, or empty when the name is not NNNN_slug.sql.
version_of() {
  case "$1" in
    [0-9][0-9][0-9][0-9]_*.sql) echo $((10#${1:0:4})) ;;
    *) echo "" ;;
  esac
}

# base_listing REF -> "name blob" lines for the .sql files under $DIR on REF.
base_listing() {
  git ls-tree "$1" -- "$DIR/" | awk '$2 == "blob" && $4 ~ /\.sql$/ { n = $4; sub(".*/", "", n); print n, $3 }'
}

max_version() {
  local max=0 v name
  while read -r name _; do
    v=$(version_of "$name")
    [ -n "$v" ] && [ "$v" -gt "$max" ] && max=$v
  done
  echo "$max"
}

check_worktree() {
  local base="$1" fail=0 listing max next name v blob prev
  git rev-parse --verify --quiet "$base^{commit}" >/dev/null || { echo "unknown base ref: $base" >&2; exit 2; }
  listing=$(base_listing "$base")
  max=$(printf '%s\n' "$listing" | max_version)
  next=$(printf '%04d' $((max + 1)))

  # Duplicates in the working tree.
  prev=""
  while read -r v name; do
    if [ -n "$prev" ] && [ "${prev%% *}" = "$v" ]; then
      printf 'duplicate version %04d: %s and %s\n' "$v" "${prev#* }" "$name"
      fail=1
    fi
    prev="$v $name"
  done < <(for f in "$DIR"/*.sql; do n=${f##*/}; v=$(version_of "$n"); [ -n "$v" ] && echo "$v $n"; done | sort -n)

  # Migrations published where this branch started (the merge-base) must
  # survive unchanged. Files that reached base after that are only "behind":
  # note them, since a merge brings them in.
  local mb mblisting
  mb=$(git merge-base HEAD "$base" 2>/dev/null || echo "$base")
  mblisting=$(base_listing "$mb")
  while read -r name _; do
    [ -z "$name" ] && continue
    [ -f "$DIR/$name" ] || printf 'note: %s is on %s but not here yet (merge %s to pick it up)\n' "$name" "$base" "$base"
  done <<<"$listing"
  while read -r name blob; do
    [ -z "$name" ] && continue
    if [ ! -f "$DIR/$name" ]; then
      printf '%s is on %s but missing here (renamed or deleted); published migrations are never changed\n' "$name" "$mb"
      fail=1
    elif [ "$(git hash-object "$DIR/$name")" != "$blob" ]; then
      printf '%s differs from %s; published migrations are never edited (fix forward with a new one)\n' "$name" "$mb"
      fail=1
    fi
  done <<<"$mblisting"

  # New migrations must come after everything on base.
  for f in "$DIR"/*.sql; do
    name=${f##*/}
    printf '%s\n' "$listing" | grep -q "^$name " && continue
    v=$(version_of "$name")
    if [ -z "$v" ]; then
      printf '%s: name must be NNNN_slug.sql\n' "$name"
      fail=1
    elif [ "$v" -le "$max" ]; then
      printf '%s: version %04d is not above %s'"'"'s newest (%04d); renumber to %s_%s\n' \
        "$name" "$v" "$base" "$max" "$next" "${name:5}"
      fail=1
      next=$(printf '%04d' $((10#$next + 1)))
    fi
  done

  if [ "$fail" -ne 0 ]; then
    echo "migration version check FAILED against $base" >&2
    return 1
  fi
  echo "migration versions OK against $base (newest there: $(printf '%04d' "$max"))"
}

report_branches() {
  local base="$1" glob="$2" listing max fail=0 b f name blob v added
  listing=$(base_listing "$base")
  max=$(printf '%s\n' "$listing" | max_version)
  echo "base $base: newest migration $(printf '%04d' "$max"); next free $(printf '%04d' $((max + 1)))"
  added=""
  for b in $(git for-each-ref --format='%(refname:short)' "refs/heads/$glob"); do
    while read -r name blob; do
      [ -z "$name" ] && continue
      printf '%s\n' "$listing" | grep -q " $blob\$" && continue # landed (same content)
      v=$(version_of "$name")
      if [ -z "$v" ]; then
        printf '%-40s %-42s bad name\n' "$b" "$name"; fail=1
      elif [ "$v" -le "$max" ]; then
        printf '%-40s %-42s RENUMBER (<= %04d on base)\n' "$b" "$name" "$max"; fail=1
      else
        printf '%-40s %-42s ok vs base\n' "$b" "$name"
      fi
      added+="$v $name $b"$'\n'
    done < <(git ls-tree "$b" -- "$DIR/" | awk '$2 == "blob" && $4 ~ /\.sql$/ { n = $4; sub(".*/", "", n); print n, $3 }')
  done
  # Same version, different file, on two branches: whichever lands second must renumber.
  local clashes
  clashes=$(printf '%s' "$added" | awk 'NF && !dup[$1 " " $2]++ { n[$1]++; files[$1] = files[$1] " " $2 }
    END { for (v in n) if (n[v] > 1) printf "version %04d is added by different files:%s\n", v, files[v] }' | sort)
  if [ -n "$clashes" ]; then
    printf '\n%s\n' "$clashes"
    fail=1
  fi
  return "$fail"
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  --branches) report_branches "${2:-$(default_base)}" "${3:-ej/*}" ;;
  *) check_worktree "${1:-$(default_base)}" ;;
esac
