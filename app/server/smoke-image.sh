#!/usr/bin/env bash
set -euo pipefail

# Mount each writing fixture outside the API image.
root=$(git rev-parse --show-toplevel)
sha=$(git rev-parse HEAD)
context=$(mktemp -d)
container=
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null
  fi
  rm -rf -- "$context"
}
trap cleanup EXIT

mkdir -p "$context/app"
case "${1:-checked-out}" in
  checked-out)
    cp -R "$root/app/content" "$context/app/"
    ;;
  renamed)
    mkdir -p "$context/app/content/writing/smoke-evening"
    printf '%s\r\n' '---' 'title: Evening' 'date: "2026-11-25"' '---' \
      'The sky darkens.' 'A lantern glows.' '' 'We head home.' \
      > "$context/app/content/writing/smoke-evening/index.md"
    ;;
  empty)
    mkdir -p "$context/app/content/writing"
    touch "$context/app/content/writing/.gitkeep"
    ;;
  *)
    echo "Usage: $0 [checked-out|renamed|empty]" >&2
    exit 2
    ;;
esac
image=$(docker build --quiet --platform linux/amd64 --build-arg "GIT_SHA=$sha" \
  -f "$root/app/server/Dockerfile" "$root")
fixture="$context/app/content"
if command -v cygpath >/dev/null; then
  fixture=$(cygpath -w "$fixture")
fi
container=$(MSYS_NO_PATHCONV=1 docker run --detach --publish 127.0.0.1::8080 \
  --mount "type=bind,source=$fixture,target=/content,readonly" "$image" serve -content-dir /content)
origin="http://$(docker port "$container" 8080/tcp)"

for ((attempt = 0; attempt < 20; attempt++)); do
  if curl -fsS "$origin/readyz" 2>/dev/null | grep -qx ok; then
    break
  fi
  sleep 1
done
curl -fsS "$origin/readyz" | grep -qx ok
curl -fsS "$origin/livez" | grep -qx ok
python3 - "$origin" "$context/app/content/writing" <<'PY'
import json
import re
import sys
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import urlopen

origin, source_dir = sys.argv[1:]
writing_url = f"{origin}/api/v1/writing"
with urlopen(writing_url, timeout=10) as response:
    items = json.load(response)

assert isinstance(items, list), items
if items:
    slug = items[0]["slug"]
    assert re.fullmatch(r"[a-z0-9]+(-[a-z0-9]+)*", slug), slug
    source = (Path(source_dir) / slug / "index.md").read_bytes().decode("utf-8-sig")
    opener, _, front_matter_and_body = source.replace("\r\n", "\n").partition("\n")
    assert opener == "---"
    lines = front_matter_and_body.split("\n")
    body = "\n".join(lines[lines.index("---") + 1:])
    with urlopen(f"{writing_url}/{slug}", timeout=10) as response:
        detail = json.load(response)
    assert detail["slug"] == slug, detail
    assert detail["body"] == body, f"Writing body differs from source for {slug}"
else:
    assert not any(path.is_dir() for path in Path(source_dir).iterdir()), "Unexpected empty list"
    try:
        with urlopen(f"{writing_url}/absent-smoke-item", timeout=10) as response:
            raise AssertionError(f"Missing writing returned {response.status}")
    except HTTPError as response:
        assert response.code == 404, response.code
        assert response.headers.get_content_type() == "application/json", response.headers
        assert json.load(response) == {
            "error": {"code": "not_found", "message": "Writing item not found."}
        }
PY
curl -fsS -D "$context/version.headers" "$origin/api/v1/version" | python3 -c '
import json, sys
assert json.load(sys.stdin)["gitSha"] == sys.argv[1]
' "$sha"
grep -Eiq '^cache-control: no-store[[:space:]]*$' "$context/version.headers"

status=$(curl -sS -H 'Accept: text/html' -o /dev/null -w '%{http_code}' "$origin/api/unknown")
[[ "$status" == 404 ]] || { echo "/api/unknown returned $status, expected 404" >&2; exit 1; }

docker stop --time 20 "$container" >/dev/null
[[ $(docker inspect --format '{{.State.ExitCode}}' "$container") == 0 ]]
