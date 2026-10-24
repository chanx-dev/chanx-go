#!/usr/bin/env bash
# Cut a release of every module in lockstep:
#
#   scripts/release.sh             # version from the commits since the last tag
#   scripts/release.sh --dry-run   # print the version and changelog, change nothing
#   scripts/release.sh 1.0.0       # force a version
#
# The version is bumped from conventional commits by git-cliff (see [bump] in
# cliff.toml). Bumps the intra-repo requires, writes CHANGELOG.md, makes a signed
# "chore(release): X.Y.Z" commit and tags the root and each nested module
# (vX.Y.Z, layers/vX.Y.Z, adapters/gin/vX.Y.Z...). Push main and the tags
# yourself; release.yml publishes the GitHub release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DRY=false
VERSION=""
for arg in "$@"; do
	case "$arg" in
	--dry-run) DRY=true ;;
	*) VERSION="${arg#v}" ;;
	esac
done

LAST="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
if [ -z "$VERSION" ]; then
	# As commitizen: only feat, fix, perf and breaking changes make a release.
	RANGE="${LAST:+$LAST..}HEAD"
	COMMITS="$(git log "$RANGE" --format='%s%n%b')"
	if ! grep -qE '^(feat|fix|perf)(\([^)]*\))?!?:|^[a-z]+(\([^)]*\))?!:|^BREAKING CHANGE' <<<"$COMMITS"; then
		echo "nothing to release: no feat, fix, perf or breaking commits since ${LAST:-the start}" >&2
		exit 1
	fi
	VERSION="$(git cliff --bumped-version 2>/dev/null)"
	VERSION="${VERSION#v}"
fi
TAG="v$VERSION"
if [ "$TAG" = "$LAST" ]; then
	echo "$TAG is already released" >&2
	exit 1
fi

if $DRY; then
	echo "next version: $TAG (last: ${LAST:-none})"
	echo
	git cliff --unreleased --tag "$TAG" --strip header 2>/dev/null
	exit 0
fi

MODULE=github.com/huynguyengl99/chanx-go
NESTED=(layers layers/redis adapters/gin adapters/echo adapters/fiber)
ALL=(. "${NESTED[@]}" examples)

if [ -n "$(git status --porcelain)" ]; then
	echo "working tree not clean" >&2
	exit 1
fi

for dir in "${ALL[@]}"; do
	for dep in "" "${NESTED[@]/#//}"; do
		path="$MODULE$dep"
		if grep -q "^\s*$path v" "$dir/go.mod" 2>/dev/null; then
			(cd "$dir" && go mod edit -require="$path@$TAG")
		fi
	done
done

git cliff --tag "$TAG" -o CHANGELOG.md
git add -A
git commit -S -m "chore(release): $VERSION"
git tag -s "$TAG" -m "$TAG"
for dir in "${NESTED[@]}"; do
	git tag -s "$dir/$TAG" -m "$dir/$TAG"
done
echo "tagged $TAG and ${#NESTED[@]} nested modules; now: git push origin main --tags"
