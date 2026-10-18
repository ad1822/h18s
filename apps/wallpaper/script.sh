#!/usr/bin/env bash
# Pick a random wallpaper from ~/Pictures/Wallpaper (including subfolders)
# and set it as the Homepage background.
#
# The homepage pod mounts ~/Pictures/Wallpaper at /app/public/images,
# so a file at <dir>/rdr2/foo.jpg is served as /images/rdr2/foo.jpg.
#
# Usage:
#   ./wallpaper.sh              random wallpaper (never the current one)
#   ./wallpaper.sh <file>       use a specific file inside the wallpaper dir
set -euo pipefail

WALL_DIR="${WALL_DIR:-$HOME/Pictures/Wallpaper}"
CONFIG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SETTINGS="${SETTINGS:-$CONFIG_DIR/settings.yaml}"
HOMEPAGE_URL="${HOMEPAGE_URL:-http://h18s}"

[[ -d "$WALL_DIR" ]] || {
  echo "wallpaper dir not found: $WALL_DIR" >&2
  exit 1
}
grep -q '^  image: ' "$SETTINGS" || {
  echo "no 'background.image' line in $SETTINGS" >&2
  exit 1
}

current="$(sed -n 's|^  image: /images/||p' "$SETTINGS")"

if [[ $# -gt 0 ]]; then
  file="$(realpath "$1")"
  root="$(realpath "$WALL_DIR")"
  [[ -f "$file" && "$file" == "$root"/* ]] || {
    echo "not a file inside $WALL_DIR: $1" >&2
    exit 1
  }
  rel="${file#"$root"/}"
else
  mapfile -d '' files < <(
    cd "$WALL_DIR" && find -L . -type f \
      \( -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.png' \
      -o -iname '*.webp' -o -iname '*.gif' -o -iname '*.avif' \) -print0
  )
  ((${#files[@]})) || {
    echo "no images in $WALL_DIR" >&2
    exit 1
  }
  for _ in 1 2 3 4 5; do
    rel="${files[RANDOM % ${#files[@]}]#./}"
    [[ ${#files[@]} -eq 1 || "$rel" != "$(python3 -c 'import sys,urllib.parse;print(urllib.parse.unquote(sys.argv[1]))' "$current")" ]] && break
  done
fi

# URL-encode each path segment (names contain spaces, commas, etc.)
url="/images/$(python3 -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.argv[1]))' "$rel")"

sed -i "s|^  image: .*|  image: $url|" "$SETTINGS"
echo "wallpaper: $rel"

# Rebuild Homepage's cached page so the change shows on the next reload
curl -fsS -m 5 "$HOMEPAGE_URL/api/revalidate" >/dev/null 2>&1 ||
  echo "note: couldn't reach $HOMEPAGE_URL/api/revalidate; reload the page manually" >&2
