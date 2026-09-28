#!/bin/sh
# Prints the CHANGELOG.md section for a release tag, as the GitHub Release body.
#
#   scripts/release-notes.sh v0.9.0 [CHANGELOG.md]
#
# The section is everything after the "## [0.9.0]" heading up to the next
# "## [" heading. The heading itself is left out, since the release title
# already carries the version. Links relative to the repository
# (docs/decisions/...) are rewritten to absolute URLs pinned to the tag, because
# a release page resolves relative links against its own URL, where they break.
#
# Fails, so the release job fails, when the section is missing or empty:
# publishing generated notes instead would hide a release cut without notes.
set -eu

tag=${1:?usage: release-notes.sh <tag> [changelog]}
changelog=${2:-CHANGELOG.md}
version=${tag#v}
repo=${GITHUB_REPOSITORY:-chebilax/sphinxor}

body=$(awk -v heading="## [$version]" '
	index($0, heading) == 1 { found = 1; next }
	found && /^## \[/ { exit }
	found { print }
' "$changelog")

# Trim leading and trailing blank lines; fail if nothing is left.
body=$(printf '%s\n' "$body" | awk 'NF { seen = 1 } seen' | awk '
	{ lines[NR] = $0 } NF { last = NR }
	END { for (i = 1; i <= last; i++) print lines[i] }
')
if [ -z "$body" ]; then
	echo "release-notes.sh: no \"## [$version]\" section in $changelog, or it is empty" >&2
	exit 1
fi

printf '%s\n' "$body" | sed -E "s#\]\(([^):\#][^):]*)\)#](https://github.com/$repo/blob/$tag/\1)#g"
