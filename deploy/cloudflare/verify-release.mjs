import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { setTimeout } from "node:timers/promises";

const [origin, dir, sha] = process.argv.slice(2);
const releaseDir = path.join(dir, "releases", sha);
const snapshot = JSON.parse(await readFile(path.join(releaseDir, "snapshot.json"), "utf8"));
const manifest = JSON.parse(await readFile(path.join(releaseDir, "manifest.json"), "utf8"));
assert.equal(snapshot.gitSha, sha);
assert.equal(manifest.gitSha, sha);
const get = (route, init = {}) => fetch(new URL(route, origin), {
  signal: AbortSignal.timeout(65_000),
  ...init,
});

let runningSHA;
for (let attempt = 0; attempt < 12; attempt += 1) {
  try {
    const response = await get("/api/v1/version");
    if (response.ok) {
      runningSHA = (await response.json()).gitSha;
      if (runningSHA === sha) break;
    }
  } catch {
    // The first API request can start a cold container.
  }
  await setTimeout(5000);
}
assert.equal(runningSHA, sha, "the public API must serve the selected release");
const list = await get("/api/v1/writing");
assert.equal(list.status, 200);
assert.deepEqual(await list.json(), snapshot.writing.map(({ body, ...summary }) => summary));
const first = snapshot.writing[0];
if (first) {
  const detail = await get("/api/v1/writing/" + first.slug);
  assert.equal(detail.status, 200);
  assert.deepEqual(await detail.json(), first);
}
const page = await get(first ? "/writing/" + first.slug : "/writing", {
  headers: { Accept: "text/html", "Sec-Fetch-Mode": "navigate" },
});
assert.equal(page.status, 200);
const pageSource = await page.text();
assert.match(pageSource, /<div id="root"><\/div>/);
assert.match(pageSource, new RegExp('<meta name="release-sha" content="' + sha + '">'));
const asset = await get(pageSource.match(/src="(\/assets\/[^"]+\.js)"/)[1]);
assert.equal(asset.status, 200);
assert.match(asset.headers.get("Content-Type"), /javascript/);
await asset.body?.cancel();
for (const [route, object] of Object.entries(manifest.media)) {
  const media = await get(route);
  assert.equal(media.status, 200, route);
  assert.equal(media.headers.get("Content-Type"), object.contentType, route);
  assert.equal(media.headers.get("Cache-Control"), "no-cache", route);
  assert.ok(media.headers.get("ETag"), route);
  const hash = createHash("sha256");
  for await (const chunk of media.body) hash.update(chunk);
  assert.equal(hash.digest("hex"), object.sha256, route);
}
const missing = await get("/media/.missing-release-verification");
assert.equal(missing.status, 404);
await missing.body?.cancel();
console.log("Verified release " + sha);
