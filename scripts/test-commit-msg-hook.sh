#!/bin/sh
# Runs .githooks/commit-msg against messages it must reject and messages it
# must accept. Part of `make check`.
set -u
hook=.githooks/commit-msg
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
fail=0

check() { # expected-exit description message
	printf '%s\n' "$3" > "$dir/msg"
	sh "$hook" "$dir/msg" 2>/dev/null
	got=$?
	if [ "$got" -ne "$1" ]; then
		echo "commit-msg hook: want exit $1, got $got — $2" >&2
		fail=1
	fi
}

# Rejected.
check 1 "trailer" "$(printf 'Fix a bug\n\nCo-Authored-By: Someone <a@b.c>')"
check 1 "trailer, lower case" "$(printf 'Fix a bug\n\nco-authored-by: Someone <a@b.c>')"
check 1 "trailer, indented" "$(printf 'Fix a bug\n\n  Co-Authored-By: Someone <a@b.c>')"
check 1 "credit" "$(printf 'Fix a bug\n\nGenerated with Claude Code')"
check 1 "credit as a markdown link" "$(printf 'Fix a bug\n\nGenerated with [Claude Code](https://claude.com/claude-code)')"
check 1 "robot emoji" "$(printf 'Fix a bug\n\n🤖 made this')"

# Accepted.
check 0 "plain message" "Fix a bug"
check 0 "'generated without' in prose" "No rule is generated without a real GuardApplication."
check 0 "'generated with' in prose" "The policy is generated with every omission flagged."
check 0 "co-authored mentioned mid-line" "Document why co-authored-by trailers are rejected."
check 0 "the hook's own wording" "Reject a Co-Authored-By line only when it is a trailer."

exit $fail
