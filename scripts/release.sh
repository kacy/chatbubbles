#!/bin/sh

set -eu

kind="${1:-}"
if [ -z "$kind" ]; then
	echo "usage: scripts/release.sh <current|patch|minor>" >&2
	exit 1
fi

repo_root="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

git update-index -q --refresh

if ! git diff --quiet --; then
	echo "tracked changes are still present; commit or stash them before releasing" >&2
	exit 1
fi

if ! git diff --cached --quiet --; then
	echo "staged changes are still present; commit or unstage them before releasing" >&2
	exit 1
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "$branch" != "main" ]; then
	echo "release targets must run from main" >&2
	exit 1
fi

current="$(tr -d '\n' < VERSION)"

case "$kind" in
	current)
		next="$current"
		;;
	patch|minor)
		old_ifs=$IFS
		IFS=.
		set -- $current
		IFS=$old_ifs

		if [ "$#" -ne 3 ]; then
			echo "VERSION must use semver like x.y.z" >&2
			exit 1
		fi

		major="$1"
		minor="$2"
		patch="$3"

		case "$kind" in
			patch)
				patch=$((patch + 1))
				;;
			minor)
				minor=$((minor + 1))
				patch=0
				;;
		esac

		next="$major.$minor.$patch"
		;;
	*)
		echo "release kind must be current, patch, or minor" >&2
		exit 1
		;;
esac

tag="v$next"
if git rev-parse --verify --quiet "$tag" >/dev/null; then
	echo "tag $tag already exists" >&2
	exit 1
fi

if [ "$kind" = "patch" ] || [ "$kind" = "minor" ]; then
	printf '%s\n' "$next" > VERSION
	git add VERSION
	git commit -m "chore: release v$next"
fi

git tag -a "$tag" -m "Release $tag"

printf 'created %s\n' "$tag"
printf 'push it with: git push origin main --follow-tags\n'
