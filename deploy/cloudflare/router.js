const shaPattern = /^[a-f0-9]{40}$/;

function error(message, status, headers = {}) {
  return new Response(message, {
    status,
    headers: { "Cache-Control": "no-store", ...headers },
  });
}

async function serveMedia(request, env, path) {
  const head = request.method === "HEAD";
  if (request.method !== "GET" && !head) {
    return error("Method not allowed.\n", 405, { Allow: "GET, HEAD" });
  }
  const sha = env.RELEASE_SHA;
  if (!shaPattern.test(sha)) throw new Error("Invalid release SHA");
  const object = await env.CONTENT.get("releases/" + sha + "/manifest.json");
  const manifest = await object?.json();
  if (manifest?.version !== 1 || manifest.gitSha !== sha ||
      !manifest.media || Array.isArray(manifest.media) || typeof manifest.media !== "object") {
    throw new Error("Invalid active media manifest");
  }
  const entry = Object.hasOwn(manifest.media, path) ? manifest.media[path] : undefined;
  if (!entry) return error(head ? null : "Media not found.\n", 404);
  if (!/^[a-f0-9]{64}$/.test(entry.sha256) || entry.key !== "blobs/" + entry.sha256 ||
      !/^(image|audio|video)\/[a-z0-9.+-]+$/.test(entry.contentType)) {
    throw new Error("Invalid active media entry");
  }
  const blob = head ? await env.CONTENT.head(entry.key) : await env.CONTENT.get(entry.key);
  if (!blob) throw new Error("Missing active media object");
  const headers = new Headers({
    "Cache-Control": "no-cache",
    "Content-Type": entry.contentType,
    "ETag": blob.httpEtag,
    "X-Content-Type-Options": "nosniff",
  });
  const tags = request.headers.get("If-None-Match")?.split(",") ?? [];
  if (tags.some((tag) => tag.trim() === "*" || tag.trim().replace(/^W\//, "") === blob.httpEtag)) {
    if (!head) await blob.body.cancel();
    return new Response(null, { status: 304, headers });
  }
  headers.set("Content-Length", String(blob.size));
  return new Response(head ? null : blob.body, { headers });
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    try {
      if (url.pathname === "/media" || url.pathname.startsWith("/media/")) {
        return await serveMedia(request, env, url.pathname);
      }
      if (url.pathname === "/api" || url.pathname.startsWith("/api/")) {
        if (env.API_TEST_ORIGIN) {
          const target = new URL(env.API_TEST_ORIGIN);
          target.pathname = url.pathname;
          target.search = url.search;
          return await fetch(new Request(target, request));
        }
        return await env.API.getByName(env.RELEASE_SHA).fetch(request);
      }
      const acceptsHTML = request.headers.get("Accept")?.split(",")
        .some((type) => type.trim().split(";")[0] === "text/html");
      const lastSegment = url.pathname.replace(/\/$/, "").split("/").at(-1);
      if ((request.method === "GET" || request.method === "HEAD") &&
          !url.pathname.startsWith("/assets/") && !lastSegment.includes(".") &&
          (acceptsHTML || request.headers.get("Sec-Fetch-Mode") === "navigate")) {
        return await env.ASSETS.fetch(new Request(new URL("/", url), request));
      }
      return await env.ASSETS.fetch(request);
    } catch (cause) {
      console.error("Release request failed", cause);
      return error(request.method === "HEAD" ? null : "Release temporarily unavailable.\n", 503);
    }
  },
};
