#!/bin/sh
set -eu

version=$(tr -d '\r\n' < VERSION)
package_version=$(node -p "require('./frontend/package.json').version")
lock_version=$(node -p "require('./frontend/package-lock.json').version")
lock_root_version=$(node -p "require('./frontend/package-lock.json').packages[''].version")

if ! printf '%s' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$'; then
  echo "VERSION 不是有效的 SemVer: $version" >&2
  exit 1
fi

for value in "$package_version" "$lock_version" "$lock_root_version"; do
  if [ "$value" != "$version" ]; then
    echo "版本不一致: VERSION=$version, package=$value" >&2
    exit 1
  fi
done

if ! grep -Fq "## [$version] -" CHANGELOG.md; then
  echo "CHANGELOG.md 缺少 $version 的正式版本记录" >&2
  exit 1
fi

if [ "${GITHUB_REF_TYPE:-}" = "tag" ] && [ "${GITHUB_REF_NAME:-}" != "v$version" ]; then
  echo "Tag ${GITHUB_REF_NAME:-<empty>} 必须与 VERSION v$version 一致" >&2
  exit 1
fi

echo "version metadata: $version"
