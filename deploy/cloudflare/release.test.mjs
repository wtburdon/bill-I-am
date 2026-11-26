import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { convertV4MiniflareOptions, Miniflare } from "miniflare";

const exec = promisify(execFile);
const root = fileURLToPath(new URL("../../", import.meta.url));
const server = path.join(root, "app/server");
const config = JSON.parse(await readFile(new URL("./wrangler.jsonc", import.meta.url), "utf8"));
const shaA = "a".repeat(40);
const shaB = "b".repeat(40);
const shaC = "c".repeat(40);
const media = "/media/writing/evening/photo.png";
const r2 = {
  R2_BUCKET: "billiam-local",
  R2_ACCESS_KEY_ID: "test-access",
  R2_SECRET_ACCESS_KEY: "test-secret",
};

test("publish, switch, and restore complete releases through Workers Static Assets and local R2", { timeout: 240_000 }, async (t) => {
  const temp = await mkdtemp(path.join(tmpdir(), "billiam-release-"));
  const cleanup = [];
  t.after(async () => {
    for (const dispose of cleanup.reverse()) await dispose();
    await rm(temp, { recursive: true, force: true });
  });
  const png = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aP7sAAAAASUVORK5CYII=",
    "base64",
  );
  const textA = "The first evening.\n\nLights ahead.\n";
  const textB = "The second evening.\n\nLights behind.\n";
  const imageA = Buffer.concat([png, Buffer.from("a")]);
  const imageB = Buffer.concat([png, Buffer.from("b")]);

  async function build(sha) {
    const binary = path.join(temp, sha[0] + (process.platform === "win32" ? ".exe" : "-api"));
    await exec("go", ["build", "-o", binary, "-ldflags", "-X main.gitSHA=" + sha, "."], { cwd: server });
    return binary;
  }
  const [apiA, apiB, apiC] = await Promise.all([build(shaA), build(shaB), build(shaC)]);
  async function buildFrontend(sha) {
    const out = path.join(temp, "client-" + sha[0]);
    await exec(process.execPath, [
      "node_modules/vite/bin/vite.js", "build", "app/client",
      "--config", "app/client/vite.config.js", "--outDir", out,
    ], { cwd: root, env: { ...process.env, VITE_RELEASE_SHA: sha } });
    return out;
  }
  const [clientA, clientB] = await Promise.all([buildFrontend(shaA), buildFrontend(shaB)]);
  async function exportRelease(name, sha, body, image) {
    const source = path.join(temp, name, "content");
    const output = path.join(temp, name, "export");
    await mkdir(path.join(source, "writing"), { recursive: true });
    if (body) {
      const item = path.join(source, "writing/evening");
      await mkdir(item);
      await writeFile(path.join(item, "index.md"), "---\ntitle: Evening\ndate: '2026-11-25'\n---\n" + body);
      await writeFile(path.join(item, "photo.png"), image);
      if (sha === shaB) await writeFile(path.join(item, "second.png"), png);
    }
    await exec(apiA, ["export", "-content-dir", source, "-release-dir", output, "-sha", sha]);
    return output;
  }
  const [exportA, exportB, exportC] = await Promise.all([
    exportRelease("release-a", shaA, textA, imageA),
    exportRelease("release-b", shaB, textB, imageB),
    exportRelease("release-c", shaC),
  ]);
  const options = {
    modules: true,
    scriptPath: path.join(root, "deploy/cloudflare/router.js"),
    compatibilityDate: config.compatibility_date,
    r2Persist: path.join(temp, "r2"),
    r2Buckets: {
      CONTENT: {
        id: r2.R2_BUCKET,
        s3Credentials: { accessKeyId: r2.R2_ACCESS_KEY_ID, secretAccessKey: r2.R2_SECRET_ACCESS_KEY },
      },
    },
    assets: {
      directory: clientA,
      binding: config.assets.binding,
      run_worker_first: config.assets.run_worker_first,
      routerConfig: { has_user_worker: true },
      assetConfig: { not_found_handling: config.assets.not_found_handling },
    },
  };
  const workerOptions = (bindings, directory = clientA) =>
    convertV4MiniflareOptions({ ...options, bindings, assets: { ...options.assets, directory } });
  const storage = new Miniflare(workerOptions({ RELEASE_SHA: shaA }));
  cleanup.push(() => storage.dispose());
  const storageURL = await storage.ready;
  const storageEnv = { ...process.env, ...r2, R2_ENDPOINT: new URL("/cdn-cgi/local/r2/s3", storageURL).href };
  const publish = (sha, dir) =>
    exec(apiA, ["publish", "-sha", sha, "-release-dir", dir], { env: storageEnv });
  const bucket = () => storage.getR2Bucket("CONTENT");
  const startAPI = async (binary, sha) => {
    const env = { ...storageEnv, R2_RELEASE_SHA: sha };
    const args = ["serve", "-addr", "127.0.0.1:0"];
    const useDocker = process.env.TEST_API_CONTAINER === "1";
    const image = "billiam-api:integration-" + sha[0];
    const name = "billiam-integration-" + process.pid + "-" + sha[0];
    if (useDocker) {
      await exec("docker", ["build", "--platform", "linux/amd64", "--build-arg", "GIT_SHA=" + sha,
        "-f", "app/server/Dockerfile", "-t", image, "."], { cwd: root });
    }
    const child = spawn(useDocker ? "docker" : binary, useDocker ? [
      "run", "--rm", "--network", "host", "--name", name,
      ...Object.keys({ ...r2, R2_ENDPOINT: "", R2_RELEASE_SHA: "" }).flatMap((key) => ["--env", key]),
      image, ...args,
    ] : args, {
      env,
      stdio: ["ignore", "ignore", "pipe"],
    });
    const exited = once(child, "exit");
    const stop = async () => {
      if (child.exitCode === null && child.signalCode === null) {
        if (useDocker) await exec("docker", ["stop", "--time", "3", name]);
        else child.kill();
      }
      await exited;
    };
    cleanup.push(stop);
    const origin = await new Promise((resolve, reject) => {
      let log = "";
      child.stderr.on("data", (chunk) => {
        log += chunk;
        const match = log.match(/Listening on (127\.0\.0\.1:\d+)/);
        if (match) resolve("http://" + match[1]);
      });
      exited.then(([code]) => reject(new Error("API exited " + code + ": " + log)), reject);
    });
    assert.equal(await (await fetch(origin + "/readyz")).text(), "ok\n");
    return { origin, stop };
  };
  const activate = async (sha, api, client) => {
    await storage.setOptions(workerOptions({ RELEASE_SHA: sha, API_TEST_ORIGIN: api.origin }, client));
    storageEnv.R2_ENDPOINT = new URL("/cdn-cgi/local/r2/s3", await storage.ready).href;
  };
  const get = (pathname, init) => storage.dispatchFetch("http://localhost" + pathname, init);
  async function assertRelease(sha, text, image) {
    const version = await get("/api/v1/version");
    const versionBody = await version.text();
    assert.equal(version.status, 200, versionBody);
    assert.equal(JSON.parse(versionBody).gitSha, sha);
    const shell = await get("/writing/evening", { headers: { "Sec-Fetch-Mode": "navigate", Accept: "text/html" } });
    assert.equal(shell.status, 200);
    assert.match(await shell.text(), new RegExp('<meta name="release-sha" content="' + sha + '">'));
    assert.equal((await (await get("/api/v1/writing/evening")).json()).body, text);
    const response = await get(media);
    assert.equal(response.status, 200);
    assert.deepEqual(Buffer.from(await response.arrayBuffer()), image);
    return response.headers.get("ETag");
  }

  await publish(shaA, exportA);
  const a = await startAPI(apiA, shaA);
  await activate(shaA, a, clientA);
  const etagA = await assertRelease(shaA, textA, imageA);
  const page = await get("/writing/evening", { headers: { "Sec-Fetch-Mode": "navigate", Accept: "text/html" } });
  assert.equal(page.status, 200);
  const html = await page.text();
  assert.match(html, /<div id="root"><\/div>/);
  const assetPath = html.match(/src="(\/assets\/[^"]+\.js)"/)[1];
  const asset = await get(assetPath);
  assert.equal(asset.status, 200);
  assert.match(asset.headers.get("Content-Type"), /javascript/);
  assert.equal((await get("/assets/missing.js")).status, 404);
  for (const missing of ["/media/writing/evening/unknown.png", "/media/writing/evening/%70hoto.png"]) {
    assert.equal((await get(missing, { headers: { "Sec-Fetch-Mode": "navigate" } })).status, 404);
  }
  assert.equal((await get("/api/unknown", { headers: { "Sec-Fetch-Mode": "navigate" } })).status, 404);

  const manifestKey = "releases/" + shaB + "/manifest.json";
  const bManifest = JSON.parse(await readFile(path.join(exportB, manifestKey), "utf8"));
  const blobKeys = Object.values(bManifest.media).map(({ key }) => key).sort();
  const secondBlob = path.join(exportB, blobKeys[1]);
  const savedBytes = await readFile(secondBlob);
  await writeFile(secondBlob, "invalid fixture bytes");
  await assert.rejects(publish(shaB, exportB), /exported blob does not match hash/);
  const afterFailure = await bucket();
  assert.ok(await afterFailure.head(blobKeys[0]));
  assert.equal(await afterFailure.head(manifestKey), null);
  assert.equal(await afterFailure.head("releases/" + shaB + "/snapshot.json"), null);
  await assertRelease(shaA, textA, imageA);
  await writeFile(secondBlob, savedBytes);
  await publish(shaB, exportB);
  await publish(shaB, exportB);
  const conflicting = await exportRelease("conflicting", shaB, "Other text.\n", imageB);
  await assert.rejects(publish(shaB, conflicting), /conflicting bytes.*snapshot/);
  const b = await startAPI(apiB, shaB);
  await activate(shaB, b, clientB);
  const etagB = await assertRelease(shaB, textB, imageB);
  assert.notEqual(etagB, etagA);
  const head = await get(media, { method: "HEAD" });
  assert.equal(head.status, 200);
  assert.equal(head.headers.get("Content-Type"), "image/png");
  assert.equal(head.headers.get("Cache-Control"), "no-cache");
  assert.equal(head.headers.get("ETag"), etagB);
  assert.equal(await head.text(), "");
  assert.equal((await get(media, { headers: { "If-None-Match": "W/" + etagB } })).status, 304);
  assert.equal((await get(media, { headers: { "If-None-Match": etagA } })).status, 200);
  await b.stop();
  assert.deepEqual(Buffer.from(await (await get(media)).arrayBuffer()), imageB);
  await activate(shaA, a, clientA);
  assert.equal(await assertRelease(shaA, textA, imageA), etagA);

  await assert.rejects(startAPI(apiC, shaC), /NoSuchKey|404|not found/i);
  const emptyKey = "releases/" + shaC + "/snapshot.json";
  const activeBucket = await bucket();
  await activeBucket.put(emptyKey, JSON.stringify({ version: 1, gitSha: shaB, writing: [] }));
  await assert.rejects(startAPI(apiC, shaC), /snapshot SHA.*does not match/);
  await activeBucket.delete(emptyKey);
  await publish(shaC, exportC);
  const c = await startAPI(apiC, shaC);
  assert.deepEqual(await (await fetch(c.origin + "/api/v1/writing")).json(), []);
  await storage.dispose();
  assert.equal((await (await fetch(a.origin + "/api/v1/writing/evening")).json()).body, textA);
});
