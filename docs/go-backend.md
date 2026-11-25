# Go backend specification

The Go service turns a Git content snapshot into a read-only HTTP API. The first release serves one collection, writing. Run the same binary behind Vite in development, behind the Worker in production, and as the full web origin in Compose and Kubernetes. The [architecture](README.md) owns Cloudflare routing and the [roadmap](roadmap.md) defines release order.

## Authoring and validation

Each direct child directory of `app/content/writing/` represents one item. Its directory name is the permanent slug and must match `^[a-z0-9]+(-[a-z0-9]+)*$`. A published directory contains a UTF-8 `index.md`. Ignore the collection's `.gitkeep`. Here is the fixture for `writing/first-light/index.md`.

```markdown
---
title: First Light
date: "2026-11-25"
summary: A morning poem.
---
First light
across the sill.

A bird sings.
```

Require a nonblank title, a real calendar date in YYYY-MM-DD form, and a nonblank body. Summary is an optional string. Preserve single line endings and blank lines in the body after the closing front matter delimiter, with CRLF normalized to LF. Dates are publication labels and carry no time zone. Readers can access an item once the release containing it is live, regardless of its date.

Use one content loader for both a proposed `validate` subcommand and server startup. Read and validate the entire collection before exposing it. Report each invalid file by repository-relative path and reason, and exit nonzero on failure. Reject duplicate slugs and symlinks in published paths. Build a read-only index keyed by collection and slug. In development, restart the service to pick up edits. An empty, valid writing directory produces an empty list.

## HTTP contract

All JSON uses UTF-8 and the camelCase field names below. Paths are case-sensitive. Support GET and HEAD, with the same status and headers and an empty body for HEAD. Other methods on known routes return 405 and an Allow header. The list is a bare array, sorted by date descending and then slug ascending.

| Path | Success response |
| --- | --- |
| `/api/v1/writing` | 200 JSON array of summaries with `slug`, `title`, `date`, and optional `summary`. Omit summary when absent. |
| `/api/v1/writing/{slug}` | 200 JSON object with the summary fields plus `body`, a Markdown string. |
| `/api/v1/version` | 200 JSON object with `gitSha`, the full source commit of the running image. Local builds may use `dev`. Send `Cache-Control: no-store`. |
| `/livez` | 200 text response once the HTTP listener can answer. |
| `/readyz` | 200 after the index is available and the service accepts traffic. Return 503 while draining. |

For example, the detail response for the fixture above is:

```json
{
  "slug": "first-light",
  "title": "First Light",
  "date": "2026-11-25",
  "summary": "A morning poem.",
  "body": "First light\nacross the sill.\n\nA bird sings.\n"
}
```

Use HTTP status to identify failure. All API errors have the shape `{"error":{"code":"not_found","message":"Writing item not found."}}` with a stable code and a human-readable message. Use `not_found` for 404, `method_not_allowed` for 405, and `internal_error` for 500. Keep filesystem paths and implementation details in server logs. Clients can ignore additional fields in v1.

## Serving and packaging

- Listen on all interfaces on port 8080 by default. Accept explicit content and client distribution directories. Development may omit the distribution when Vite serves the client. Inject the release SHA at build time and include it in startup logs.
- Reserve the API, media, and probe routes before resolving frontend files. Return 404 for unknown API routes, unpublished media, and missing asset files. When serving the built client, return `index.html` for HTML document navigation to a client route. A refresh of `/writing/first-light` must work in the standalone image.
- Send `Cache-Control: no-cache` on content responses, mutable media, and the HTML shell. Content-hashed frontend assets may use long-lived immutable caching. Shut down gracefully on SIGTERM so pending reads can finish.
- During the gallery milestone, read ordered `index.json` entries, validate nonblank alt text and file existence, and serve only indexed media keys at `/media/{collection}/{slug}/{filename}`. Resolve filenames within the item directory and send the matching MIME type. In later collections, keep list summaries small and put ordered images, project links, or a registered game module key on the detail response.

## Acceptance

1. Run validation against the poem fixture and against a copy with an impossible date. The valid fixture succeeds and the invalid one identifies its file and fails both validation and startup.
2. Exercise list, detail, and empty collection responses with Go HTTP tests. Use two same-day items to verify the tie-breaker. Check the exact poem body, missing slug, and 405 response.
3. Start the release image and fetch a deep link with an HTML Accept header, its JS asset, and the JSON detail endpoint. Check that a missing API, media, or JS file returns 404 even when requested by a browser.
4. Verify both probes and compare the uncacheable version response with the deployed Git SHA. After restoring a release, verify both its SHA and its poem text.
