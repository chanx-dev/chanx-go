#!/usr/bin/env bash
# Cut a release of every module that changed, each at its own version:
#
#   scripts/release.sh                 # versions from the commits since each module's last tag
#   scripts/release.sh --dry-run       # print the versions and changelogs, change nothing
#   scripts/release.sh layers=1.0.0    # force a module's version (root is ".")
#   scripts/release.sh --notes TAG     # print the release notes of an existing tag
#
# A module releases when a feat, fix, perf or breaking commit touched its directory
# since its last tag (vX.Y.Z for the root, layers/vX.Y.Z...). The bump follows
# commitizen: fix and perf -> patch, feat -> minor, a breaking change -> major, or minor
# while on 0.x so v1.0.0 is only cut on purpose.
#
# Every go.mod then requires the latest version of this repo's modules, each released
# module gets <dir>/CHANGELOG.md, and one signed "chore(release): ..." commit carries
# them all, with one signed tag per released module. Push main, then each tag on its
# own: GitHub starts no workflow for a push of more than three tags, and every tag
# runs release.yml.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

MODULE=go.chanx.dev
# In dependency order, so a dependency is always tagged before what requires it.
MODULES=(layers layers/redis . adapters/gin adapters/echo adapters/fiber)
GOMODS=("${MODULES[@]}" examples docs)

# Paths a module's history ignores: the modules nested inside it, and the examples
# module and go.work, which are never released.
excludes() {
	case "$1" in
	.) echo layers adapters examples go.work ;;
	layers) echo layers/redis ;;
	esac
}

prefix() { [ "$1" = . ] && echo "v" || echo "$1/v"; }
import_path() { [ "$1" = . ] && echo "$MODULE" || echo "$MODULE/$1"; }

last_tag() { git describe --tags --abbrev=0 --match "$(prefix "$1")[0-9]*" "${2:-HEAD}" 2>/dev/null || true; }

pathspec() {
	local ex
	echo "$1"
	for ex in $(excludes "$1"); do echo ":!$ex"; done
}

cliff_paths() {
	local ex
	if [ "$1" = . ]; then echo "--include-path=**"; else echo "--include-path=$1/**"; fi
	for ex in $(excludes "$1"); do echo "--exclude-path=$ex" "--exclude-path=$ex/**"; done
}

next_version() {
	local dir="$1" last="$2" commits major minor patch
	commits="$(git log "${last:+$last..}HEAD" --format='%s%n%b' -- $(pathspec "$dir"))"
	if [ -z "$last" ]; then
		echo 0.1.0
		return
	fi
	IFS=. read -r major minor patch <<<"${last##*v}"
	if grep -qE '^[a-z]+(\([^)]*\))?!:|^BREAKING CHANGE' <<<"$commits"; then
		[ "$major" = 0 ] && echo "0.$((minor + 1)).0" || echo "$((major + 1)).0.0"
	elif grep -qE '^feat(\([^)]*\))?:' <<<"$commits"; then
		echo "$major.$((minor + 1)).0"
	elif grep -qE '^(fix|perf)(\([^)]*\))?:' <<<"$commits"; then
		echo "$major.$minor.$((patch + 1))"
	fi
}

# git-cliff drops a tag whose commit is outside --include-path (a release commit that
# never touched the module), so each section is rendered from an explicit range. A
# lone commit id is the range from the first commit; a tag name there is refused.
section() {
	local dir="$1" from="$2" to="$3" tag="$4" range
	range="$(git rev-parse "$to^{commit}")"
	[ -n "$from" ] && range="$from..$range"
	git cliff "$range" $(cliff_paths "$dir") --tag "$tag" --strip header 2>/dev/null
}

changelog() {
	local dir="$1" next="$2" tags prev="" tag out=""
	tags="$(git tag --list "$(prefix "$dir")[0-9]*" --sort=v:refname)"
	for tag in $tags; do
		out="$(section "$dir" "$prev" "$tag" "$tag")${out:+$'\n\n'$out}"
		prev="$tag"
	done
	if [ -n "$next" ]; then
		out="$(section "$dir" "$prev" HEAD "$next")${out:+$'\n\n'$out}"
	fi
	printf '# Changelog\n\n%s\n' "$out"
}

if [ "${1:-}" = --notes ]; then
	tag="${2:?usage: release.sh --notes TAG}"
	dir="${tag%v*}"
	dir="${dir%/}"
	dir="${dir:-.}"
	section "$dir" "$(last_tag "$dir" "$tag^")" "$tag" "$tag"
	exit 0
fi

DRY=false
FORCED=()
for arg in "$@"; do
	case "$arg" in
	--dry-run) DRY=true ;;
	*=*) FORCED+=("$arg") ;;
	*) echo "unknown argument: $arg" >&2 && exit 2 ;;
	esac
done

forced() {
	local arg
	for arg in ${FORCED[@]+"${FORCED[@]}"}; do
		[ "${arg%%=*}" = "$1" ] && echo "${arg#*=}"
	done
	return 0
}

# LAST[i], NEXT[i] and TAG[i] belong to MODULES[i]; NEXT is empty when it does not release.
LAST=() NEXT=() TAG=()
for i in "${!MODULES[@]}"; do
	dir="${MODULES[$i]}"
	LAST[i]="$(last_tag "$dir")"
	version="$(forced "$dir")"
	version="${version:-$(next_version "$dir" "${LAST[i]}")}"
	NEXT[i]="${version#v}"
	TAG[i]="$(prefix "$dir")${NEXT[i]}"
	if [ -n "${NEXT[i]}" ] && [ "${TAG[i]}" = "${LAST[i]}" ]; then
		echo "${TAG[i]} is already released" >&2
		exit 1
	fi
done

if [ -z "$(printf '%s' "${NEXT[@]}")" ]; then
	echo "nothing to release: no feat, fix, perf or breaking commits since the last tags" >&2
	exit 1
fi

if $DRY; then
	for i in "${!MODULES[@]}"; do
		if [ -n "${NEXT[i]}" ]; then
			printf '%-16s %s -> %s\n' "${MODULES[$i]}" "${LAST[i]:-none}" "${TAG[i]}"
		fi
	done
	for i in "${!MODULES[@]}"; do
		if [ -n "${NEXT[i]}" ]; then
			echo
			section "${MODULES[$i]}" "${LAST[i]}" HEAD "${TAG[i]}"
		fi
	done
	exit 0
fi

if [ -n "$(git status --porcelain)" ]; then
	echo "working tree not clean" >&2
	exit 1
fi

for gomod in "${GOMODS[@]}"; do
	for i in "${!MODULES[@]}"; do
		path="$(import_path "${MODULES[$i]}")"
		version="${NEXT[i]:-${LAST[i]##*v}}"
		if [ -n "$version" ] && grep -q "^\s*$path v" "$gomod/go.mod"; then
			(cd "$gomod" && go mod edit -require="$path@v$version")
		fi
	done
done

SUMMARY="" TAGS=""
for i in "${!MODULES[@]}"; do
	[ -z "${NEXT[i]}" ] && continue
	dir="${MODULES[$i]}"
	changelog "$dir" "${TAG[i]}" >"$dir/CHANGELOG.md"
	SUMMARY="${SUMMARY:+$SUMMARY, }$([ "$dir" = . ] && echo chanx || echo "$dir") ${NEXT[i]}"
	TAGS="${TAGS:+$TAGS }${TAG[i]}"
done

git add -A
git commit -S -m "chore(release): $SUMMARY"
for tag in $TAGS; do
	git tag -s "$tag" -m "$tag"
done
echo "tagged $TAGS; now:"
echo "  git push origin main"
echo "  for t in $TAGS; do git push origin \"\$t\"; done"
