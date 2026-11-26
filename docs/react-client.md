# React client specification

A reader can land on a poem from a shared link, read its intended line breaks, and find the rest of Billy's writing. Build the first release in `app/client/` with React, TypeScript, and Vite. The [Go backend specification](go-backend.md) defines response fields, status codes, and a sample poem. The [roadmap](roadmap.md) schedules galleries, project pages, and games after writing.

## Pages and navigation

| URL | First-release behavior |
| --- | --- |
| `/` | Show the site name, a short introduction, and a link to Writing. Add section links as each collection becomes available. |
| `/writing` | Fetch `GET /api/v1/writing`. Display titles as permanent links in response order, with publication dates and summaries when supplied. An empty array displays an empty collection message. |
| `/writing/{slug}` | Fetch `GET /api/v1/writing/{slug}` directly. Display the title, date, Markdown body, and a link back to Writing. This page works as the first URL of a visit. |
| Unknown client route | Show a page explaining that the address could not be found, with a link to the homepage. |

Use normal links so copying an address, opening a new tab, browser Back and Forward, and refreshing a deep link work. Keep the site header and navigation available during loading and failure. Set the document title from the active page and move focus to the main heading on client-side page navigation.

## Reading and data handling

- Use same-origin relative URLs. Vite proxies `/api` to the local Go port. The release build emits a static distribution for Workers Static Assets. Use the local Worker integration setup to exercise `/media` with R2 emulation.
- Treat the writing list as an array and a detail response as one object. Use `body` as Markdown and `slug` as the link identifier. Format the date as a calendar day so a reader's time zone cannot shift publication to the day before.
- Render single newlines within verse as visible line breaks and blank lines as stanza breaks. Preserve authored Markdown emphasis, links, and lists. Disable raw HTML and allow safe link schemes. Keep the reading column comfortably narrow and allow long lines to wrap on phones.
- Give each route distinct loading, success, missing, and temporary failure states. A 404 on a detail request offers a link to Writing. Network errors, invalid successful payloads, and 5xx responses offer Retry on the current route. Cancel or disregard stale requests on route change so a slow response for one poem cannot replace another.
- Use semantic landmarks, a single page heading, visible keyboard focus, and descriptive link text. Announce asynchronous status changes to assistive technology. Keep navigation usable at narrow widths and when text is enlarged.

## Later sections

Use the list/detail URL pattern for `/photography`, `/drawings`, `/projects`, and `/games`. A gallery preserves the server's image order, alt text, and captions. Use responsive thumbnails linking to full-size views with keyboard navigation and predictable return focus. Project detail supports a case study, repository link, and live demo when provided.

Game detail shows instructions and credits. Resolve its game module key through an explicit client-side registry and import the game on demand. An unknown key leaves the description readable and shows an unavailable state. Leaving the game releases animation frames, timers, input handlers, and audio.

## Acceptance

1. Build using the lockfile and run TypeScript checks in pull requests. Verify that the Vite proxy and the built site can load a published fixture from a fresh direct visit.
2. Open Writing, follow the poem, refresh, and navigate Back. Confirm title, date, one verse line break, and one stanza break. Read the page at a narrow viewport and follow every navigation link using the keyboard.
3. Exercise an empty collection, a missing poem, and a failed request followed by Retry. Delay a detail response while navigating away and confirm the new page retains its own content.
4. For later milestones, inspect the request log to confirm reading a poem downloads no game code. Open a game and return to a gallery, then verify its input handlers and animation have stopped.
