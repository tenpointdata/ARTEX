#!/usr/bin/env bash
# Tests for scripts/check-migration-versions.sh, run in a throwaway git repo.
set -euo pipefail
src="$(cd "$(dirname "$0")" && pwd)/check-migration-versions.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
git init -q -b main .
git config user.email t@example.invalid
git config user.name t
mkdir -p scripts db/migrations
cp "$src" scripts/
echo 'SELECT 1;' >db/migrations/0001_a.sql
echo 'SELECT 2;' >db/migrations/0003_b.sql
git add . && git commit -qm base

pass=0
expect() { # expect CODE DESCRIPTION -- CMD...
  local want=$1 desc=$2 got=0
  shift 3
  "$@" >out.txt 2>&1 || got=$?
  if [ "$got" -ne "$want" ]; then
    printf 'FAIL: %s (exit %s, want %s)\n' "$desc" "$got" "$want"; cat out.txt; exit 1
  fi
  pass=$((pass + 1))
}
check() { scripts/check-migration-versions.sh main; }

expect 0 "unchanged tree passes" -- check
echo 'SELECT 4;' >db/migrations/0004_c.sql
expect 0 "new migration above base max passes" -- check
echo 'SELECT 2;' >db/migrations/0002_gap.sql
expect 1 "new migration filling a gap below max fails" -- check
grep -q 'renumber to 0004_gap.sql' out.txt || { echo "FAIL: no renumber hint"; cat out.txt; exit 1; }
rm db/migrations/0002_gap.sql
echo 'SELECT 5;' >db/migrations/0004_d.sql
expect 1 "duplicate version in tree fails" -- check
rm db/migrations/0004_d.sql
echo '-- edited' >>db/migrations/0003_b.sql
expect 1 "edited published migration fails" -- check
git checkout -q db/migrations/0003_b.sql
git mv db/migrations/0003_b.sql db/migrations/0005_b.sql
expect 1 "renamed published migration fails" -- check
git mv db/migrations/0005_b.sql db/migrations/0003_b.sql
rm db/migrations/0004_c.sql

# --branches: two branches adding 0004 clash; a branch carrying a landed file is fine.
git checkout -qb ej/one && echo 'SELECT 41;' >db/migrations/0004_one.sql && git add . && git commit -qm one
git checkout -q main && git checkout -qb ej/two && echo 'SELECT 42;' >db/migrations/0004_two.sql && git add . && git commit -qm two
git checkout -q main
expect 1 "two branches adding the same version are reported" -- scripts/check-migration-versions.sh --branches main
grep -q 'version 0004 is added by different files' out.txt || { echo "FAIL: clash not named"; cat out.txt; exit 1; }
git branch -qD ej/two
expect 0 "a single branch above base is ok" -- scripts/check-migration-versions.sh --branches main

# A branch behind main is noted, not failed; its own new migration must clear main's tip.
git checkout -qb ej/behind
git checkout -q main && echo 'SELECT 5;' >db/migrations/0005_main.sql && git add . && git commit -qm main5
git checkout -q ej/behind
expect 0 "a branch behind base passes with a note" -- check
grep -q 'note: 0005_main.sql' out.txt || { echo "FAIL: behind not noted"; cat out.txt; exit 1; }
echo 'SELECT 55;' >db/migrations/0005_mine.sql
expect 1 "a behind branch reusing base's new version fails" -- check
grep -q 'renumber to 0006_mine.sql' out.txt || { echo "FAIL: wrong hint"; cat out.txt; exit 1; }

printf 'check-migration-versions: %d tests passed\n' "$pass"
