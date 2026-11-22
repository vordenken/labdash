#!/bin/bash
set -euo pipefail

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

case "${1:-}" in
  check)
    : "${EVENT_NAME:?}" "${REF_NAME:?}" "${HEAD_SHA:?}" "${GITHUB_OUTPUT:?}"
    case "$REF_NAME" in
      main|feature/*) ;;
      *) echo "should-build=false" >> "$GITHUB_OUTPUT"; exit 0 ;;
    esac
    should_build=true
    if [[ "$EVENT_NAME" != workflow_dispatch ]]; then
      : "${BEFORE:?}"
      if [[ "$BEFORE" == 0000000000000000000000000000000000000000 && "$REF_NAME" == feature/* ]]; then
        BEFORE="$(git merge-base "$HEAD_SHA" origin/main)" \
          || fail "Cannot find the common ancestor with main."
      fi
      if [[ "$BEFORE" != 0000000000000000000000000000000000000000 ]]; then
        git cat-file -e "$BEFORE^{commit}" || fail "Cannot find previous commit $BEFORE."
        if [[ "$REF_NAME" == main ]]; then
          if git cat-file -e "$BEFORE:semver.txt" 2>/dev/null; then
            previous="$(git show "$BEFORE:semver.txt" | head -n 1)"
            current="$(head -n 1 semver.txt)"
            [[ "$previous" != "$current" ]] || should_build=false
          fi
        else
          if git diff --quiet "$BEFORE" "$HEAD_SHA" -- . \
            ':(exclude,glob)**/*.md' ':(exclude)LICENSE' ':(exclude).gitignore' \
            ':(exclude).github/ISSUE_TEMPLATE' ':(exclude).github/dependabot.yml' \
            ':(exclude).github/pull_request_template.md' ':(exclude)tests' \
            ':(exclude)features' ':(exclude,glob)**/*_test.go'; then
            should_build=false
          else
            status=$?
            [[ "$status" == 1 ]] || fail "Cannot compare $BEFORE with $HEAD_SHA."
          fi
        fi
      fi
    fi
    echo "should-build=$should_build" >> "$GITHUB_OUTPUT"
    ;;
  prepare)
    : "${HEAD_SHA:?}" "${REF_NAME:?}" "${GITHUB_OUTPUT:?}"
    version="$(head -n 1 semver.txt)"
    [[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] \
      || fail "Invalid version '$version' in semver.txt."
    [[ "$(sed -n 2p semver.txt)" == "---" ]] || fail "semver.txt must have --- on line 2."
    notes="$(awk 'NR <= 2 { next } /^## [0-9]+\.[0-9]+\.[0-9]+/ { exit } { print }' semver.txt)"
    [[ "$notes" =~ [^[:space:]] ]] || fail "semver.txt has no current release notes."
    case "$REF_NAME" in
      main)
        tag="v$version"
        prerelease=false
        ;;
      feature/*)
        latest=0
        retry=0
        tags="$(git tag --list "v$version-beta*")"
        for candidate in $tags; do
          number="${candidate#"v$version-beta"}"
          [[ "$number" =~ ^[1-9][0-9]*$ ]] || continue
          if (( number > latest )); then latest="$number"; fi
          if [[ "$(git rev-list -n 1 "$candidate")" == "$HEAD_SHA" ]] && (( number > retry )); then
            retry="$number"
          fi
        done
        if (( retry > 0 )); then number="$retry"; else number=$((latest + 1)); fi
        tag="v$version-beta$number"
        prerelease=true
        ;;
      *) fail "Releases require main or feature/*, not '$REF_NAME'." ;;
    esac
    if git show-ref --verify --quiet "refs/tags/$tag"; then
      [[ "$(git rev-list -n 1 "$tag")" == "$HEAD_SHA" ]] \
        || fail "Tag $tag belongs to a different commit; refusing to move it."
    fi
    {
      echo "tag=$tag"
      echo "version=$version"
      echo "image-version=${tag#v}"
      echo "major=${version%%.*}"
      echo "major-minor=${version%.*}"
      echo "prerelease=$prerelease"
    } >> "$GITHUB_OUTPUT"
    ;;
  *) fail "Usage: bash scripts/release.sh {check|prepare}" ;;
esac
