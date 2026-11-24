#!/bin/sh
# bbot reads everything it was installed with from the caller's home: its tools
# from $HOME/.bbot/tools, its config from $HOME/.config/bbot/bbot.yml and the
# "core deps done" marker from $HOME/.bbot/cache (bbot 3.0.2 has no environment
# override; core/config/files.py uses Path.home()). The image installs all of it
# into root's home, but agent processes run as the executor with a fresh per-run
# HOME (server/execpriv_home_unix.go), where bbot would find no tools, no config,
# and would run `ansible-galaxy collection install community.general` (network)
# on every scan.
#
#   bbot-home.sh seed SEED_DIR PYTHON   (build, as root) snapshot root's bbot home
#                                        into a read-only seed directory
#   bbot-home.sh exec SEED_DIR BBOT ... (runtime) copy the seed into $HOME without
#                                        overwriting anything, then exec bbot
#   bbot-home.sh check SEED_DIR UID     (build, as root) scan a loopback HTTP server
#                                        as UID from an empty home, no network needed
#
# The seed holds links to the installed tools (read-only, root-owned), the
# core-deps marker, the module setup status and bbot.yml. Copying runs as the
# caller, into the caller's own home, so it grants nothing the caller lacked.
set -eu

mode="${1:?usage: bbot-home.sh seed|exec SEED_DIR ...}"
seed="${2:?seed directory required}"
shift 2

case "$mode" in
seed)
    python="${1:?bbot python required}"
    src=/root/.bbot
    tools="$(readlink -f "$src/tools")"
    test -d "$tools"
    marker="$("$python" -c 'import mmh3, orjson
from bbot.core.helpers.depsinstaller.installer import DepsInstaller as D
print(mmh3.hash(orjson.dumps(D.CORE_DEPS, option=orjson.OPT_SORT_KEYS)))')"
    # The marker is only there when bbot's core deps were satisfied at build.
    test -e "$src/cache/$marker"
    rm -rf "$seed"
    mkdir -p "$seed/.bbot/cache/depsinstaller" "$seed/.config/bbot"
    # bbot checks at start that its tools directory is writable, so each home
    # gets a directory of its own holding one link per installed tool. bbot
    # writes sudo_askpass.py there itself; a link would point it at root's copy.
    mkdir "$seed/.bbot/tools"
    for entry in "$tools"/* "$tools"/.[!.]*; do
        [ -e "$entry" ] || continue
        case "${entry##*/}" in sudo_askpass.py) continue ;; esac
        ln -s "$entry" "$seed/.bbot/tools/${entry##*/}"
    done
    : >"$seed/.bbot/cache/$marker"
    if [ -f "$src/cache/depsinstaller/setup_status.json" ]; then
        cp "$src/cache/depsinstaller/setup_status.json" "$seed/.bbot/cache/depsinstaller/"
    fi
    if [ -f /root/.config/bbot/bbot.yml ]; then
        cp /root/.config/bbot/bbot.yml "$seed/.config/bbot/bbot.yml"
    else
        : >"$seed/.config/bbot/bbot.yml"
    fi
    chown -R -h root:root "$seed"
    chmod -R a+rX,go-w "$seed"
    ;;
exec)
    bbot="${1:?bbot binary required}"
    shift
    case "${HOME:-/}" in
    / | "") ;;
    *)
        if [ -d "$seed" ] && ! cp -R -P -n "$seed/." "$HOME/"; then
            echo "bbot-home: could not seed $HOME from $seed; module tools may be missing" >&2
        fi
        ;;
    esac
    exec "$bbot" "$@"
    ;;
check)
    uid="${1:?executor uid required}"
    work="$(mktemp -d)"
    mkdir -p "$work/www" "$work/run"
    echo '<html><title>bbot-home check</title></html>' >"$work/www/index.html"
    (cd "$work/www" && exec python3 -m http.server 18765 --bind 127.0.0.1) >/dev/null 2>&1 &
    server=$!
    trap 'kill "$server" 2>/dev/null || :; rm -rf "$work"' EXIT
    chown "$uid:$uid" "$work/run"
    chmod 0755 "$work"
    sleep 1
    setpriv --reuid="$uid" --regid="$uid" --clear-groups \
        env -i PATH="$PATH" HOME="$work/run/home" sh -c 'mkdir "$HOME" && cd "$HOME/.." &&
            bbot -t http://127.0.0.1:18765/ -m http,gowitness,fingerprintx \
                -om json -o out -n check -y --no-deps --no-color' >"$work/log" 2>&1 ||
        { cat "$work/log"; exit 1; }
    out="$work/run/out/check/output.json"
    for event in WEBSCREENSHOT PROTOCOL; do
        grep -q "\"type\": *\"$event\"" "$out" || { cat "$work/log"; echo "no $event event" >&2; exit 1; }
    done
    if grep -qiE 'ansible|galaxy|Installing' "$work/log"; then
        cat "$work/log"
        echo "bbot tried to install dependencies" >&2
        exit 1
    fi
    ;;
*)
    echo "bbot-home.sh: unknown mode $mode" >&2
    exit 2
    ;;
esac
