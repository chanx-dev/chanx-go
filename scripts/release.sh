#!/usr/bin/env bash
# Cut a release of every module in lockstep:
#
#   scripts/release.sh 0.1.0
#
# Bumps the intra-repo requires to the new version, writes CHANGELOG.md with
# git-cliff, makes a signed "chore(release): X.Y.Z" commit and tags the root and
# each nested module (vX.Y.Z, layers/vX.Y.Z, adapters/gin/vX.Y.Z...). Push main
# and the tags yourself; release.yml publishes the GitHub release.
set -euo pipefail

VERSION="${1:?usage: scripts/release.sh X.Y.Z}"
TAG="v$VERSION"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

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
