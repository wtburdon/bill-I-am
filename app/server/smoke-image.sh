#!/usr/bin/env bash
set -euo pipefail

# Build in a temporary context so the smoke client cannot replace a local Vite build.
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

mkdir -p "$context/app/client/dist/assets"
cp -R "$root/app/server" "$root/app/content" "$context/app/"
cat > "$context/app/client/dist/index.html" <<'HTML'
<!doctype html><title>Backend smoke test</title><script type="module" src="/assets/smoke-D3Lkc82f.js"></script>
HTML
printf '%s\n' 'console.log("backend smoke test")' > "$context/app/client/dist/assets/smoke-D3Lkc82f.js"

image=$(docker build --quiet --platform linux/amd64 --build-arg "GIT_SHA=$sha" \
  -f "$context/app/server/Dockerfile" "$context")
container=$(docker run --detach --publish 127.0.0.1::8080 "$image")
origin="http://$(docker port "$container" 8080/tcp)"

for ((attempt = 0; attempt < 20; attempt++)); do
  if curl -fsS "$origin/readyz" 2>/dev/null | grep -qx ok; then
    break
  fi
  sleep 1
done
curl -fsS "$origin/readyz" | grep -qx ok
curl -fsS "$origin/livez" | grep -qx ok
curl -fsS -H 'Accept: text/html' "$origin/writing/first-light" | grep -Fq '<title>Backend smoke test</title>'
curl -fsS "$origin/assets/smoke-D3Lkc82f.js" | grep -Fq 'console.log'
curl -fsS "$origin/api/v1/writing/first-light" | python3 -c '
import json, sys
from pathlib import Path
source = Path(sys.argv[1]).read_text(encoding="utf-8-sig")
body = source.split("\n---\n", 1)[1]
assert json.load(sys.stdin)["body"] == body
' "$context/app/content/writing/first-light/index.md"
curl -fsS -D "$context/version.headers" "$origin/api/v1/version" | python3 -c '
import json, sys
assert json.load(sys.stdin)["gitSha"] == sys.argv[1]
' "$sha"
grep -Eiq '^cache-control: no-store[[:space:]]*$' "$context/version.headers"

for path in /api/unknown /media/writing/first-light/image.jpg /assets/missing.js; do
  status=$(curl -sS -H 'Accept: text/html' -o /dev/null -w '%{http_code}' "$origin$path")
  [[ "$status" == 404 ]] || { echo "$path returned $status, expected 404" >&2; exit 1; }
done

docker stop --time 20 "$container" >/dev/null
[[ $(docker inspect --format '{{.State.ExitCode}}' "$container") == 0 ]]
