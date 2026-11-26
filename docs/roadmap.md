# Platform roadmap

**First public release:** a reader can follow a link directly to a poem, and Billy can publish a revision by merging a content-only pull request. The release is complete when GitHub Actions verifies the revision on Cloudflare and a previous release can be restored. See the [architecture](README.md) for repository boundaries, HTTP routes, and deployment design.

| Order | Deliverable | How to verify it |
| --- | --- | --- |
| 1. Publish one poem locally | Add a real or sample poem under `app/content/writing/`. Implement the Go loader, list/detail handlers, and React writing index and reader. Add Vite's local proxy. Start PR checks for the client build, Go tests, and content validation. | Open and refresh the poem's URL. Stanzas retain their spacing. An unknown slug returns a 404 from Go and a helpful page in React. Editing the Markdown updates the reading view after reload and content reindexing. |
| 2. Package the release | Use the API image, Compose mounts, health endpoints, and embedded release SHA. Build Vite separately. | From a fresh checkout, run the API through Compose and open the site through Vite. A missing API route returns 404. A malformed content file fails validation before deployment. |
| 3. Ship the first public release | Publish each Git SHA to private R2 and deploy matching Worker assets and a digest-pinned API image. Run the staging drills in the [release runbook](../deploy/cloudflare/README.md). | Deploy two revisions with different writing and media. Verify both and restore the earlier SHA, writing, and media together. |
| 4. Publish the portfolio | Add photography and drawing collections, responsive gallery thumbnails, full-size views, alt text, and project pages. Choose a permanent hostname before widely sharing links. | Publish one gallery and one project from Git. Navigate the gallery by keyboard, open a project on a phone, and follow a shared gallery URL in a fresh browser. CI catches a broken image reference. |
| 5. Ship a browser game | Add a game catalog entry and one lazy-loaded game route. Keep game code in the client and instructions, credits, and cover art in content. | A direct game URL loads the game and its instructions. Visiting a poem does not fetch the game bundle. Returning to the gallery stops the game's animation and input handlers. |

## Kubernetes learning track

Start after milestone 2, as soon as the portable image passes its smoke tests. The production release can proceed independently.

1. Create a local kind cluster, load a commit-tagged image, and add a Deployment and ClusterIP Service under `deploy/kubernetes/`. Port-forward the Service and run the same HTTP checks as Compose. [kind quick start](https://kind.sigs.k8s.io/docs/user/quick-start/)
2. Add readiness and liveness probes against Go, set CPU and memory requests and limits, and scale to two replicas. Delete one pod, watch its replacement become ready, and verify the poem remains available through the Service.
3. Roll out a second image containing an edited poem. Observe rollout status and the served SHA. Try a release that fails readiness and practice undoing it. Finish when the earlier SHA and content are serving again. Record the commands that worked in the manifests directory.

## Later decisions

- **Add Minecraft hosting** when edition, audience, and hosting budget are known. Give the game its own hostname, persistent world, backups, and restore drill. Website deploys should be possible while players are online.

The Go image smoke tests discover published writing dynamically. CI also tests a renamed poem and an empty collection. The Worker integration runs its own releases to test content changes and rollback independently of the published sample.
