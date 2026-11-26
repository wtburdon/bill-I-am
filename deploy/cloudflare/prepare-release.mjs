import { readFileSync, writeFileSync } from "node:fs";

const [sha, image, configPath, secretsPath] = process.argv.slice(2);
const { WORKER_NAME, R2_BUCKET, R2_ENDPOINT, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY } = process.env;
if (!/^[a-f0-9]{40}$/.test(sha) ||
    !/^registry\.cloudflare\.com\/[a-f0-9]+\/[a-z0-9-]+@sha256:[a-f0-9]{64}$/.test(image)) {
  throw new Error("Expected a full Git SHA and a digest-pinned Cloudflare registry image");
}
if (!WORKER_NAME || !R2_BUCKET || !R2_ENDPOINT || !R2_ACCESS_KEY_ID || !R2_SECRET_ACCESS_KEY) {
  throw new Error("Missing release configuration or read credentials");
}
const local = JSON.parse(readFileSync(new URL("./wrangler.jsonc", import.meta.url), "utf8"));
writeFileSync(configPath, JSON.stringify({
  ...local,
  name: WORKER_NAME,
  main: "./index.js",
  workers_dev: true,
  preview_urls: false,
  vars: { RELEASE_SHA: sha, R2_BUCKET, R2_ENDPOINT },
  r2_buckets: [{ binding: "CONTENT", bucket_name: R2_BUCKET }],
  durable_objects: { bindings: [{ name: "API", class_name: "WritingAPI" }] },
  exports: { WritingAPI: { type: "durable-object", storage: "sqlite" } },
  containers: [{
    class_name: "WritingAPI",
    scheduling_policy: "durable_object",
    images: { api: { image } },
  }],
}, null, 2) + "\n");
writeFileSync(secretsPath, JSON.stringify({ R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY }), { mode: 0o600 });
