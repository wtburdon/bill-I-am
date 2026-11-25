# Platform roadmap

**First public release:** a reader can follow a link directly to a poem, and Billy can publish a revision by merging a content-only pull request. The release is complete when GitHub Actions verifies the revision on Cloudflare and a previous release can be restored. See the [architecture](README.md) for repository boundaries, HTTP routes, and deployment design.

| Order | Deliverable | How to verify it |
| --- | --- | --- |
| 1. Publish one poem locally | Add a real or sample poem under `app/content/writing/`. Implement the Go loader, list/detail handlers, and React writing index and reader. Add Vite's local proxy. Start PR checks for the client build, Go tests, and content validation. | Open and refresh the poem's URL. Stanzas retain their spacing. An unknown slug returns a 404 from Go and a helpful page in React. Editing the Markdown updates the reading view after reload and content reindexing. |
| 2. Package the release | Add the multi-stage Dockerfile, Compose service, health endpoints, and embedded release SHA. Extend CI to build the image and run HTTP smoke tests. | From a fresh checkout, `docker compose -f deploy/compose.yaml up --build` serves the poem and React assets from one port. A missing API route returns 404. A malformed content file fails validation before deployment. |
| 3. Ship the first public release | Add the Worker, asset routing, named container binding, and main-branch deploy and recovery jobs. Run the first deploy on a Cloudflare-provided hostname. Record account prerequisites and the verified release in GitHub. | Merge a one-line poem edit. The public deep link shows it, the API reports the merge SHA, and an unknown media URL returns 404. Restore the previous release, verify the old line and SHA, then promote the new release again. |
| 4. Publish the portfolio | Add photography and drawing collections, responsive gallery thumbnails, full-size views, alt text, and project pages. Choose a permanent hostname before widely sharing links. | Publish one gallery and one project from Git. Navigate the gallery by keyboard, open a project on a phone, and follow a shared gallery URL in a fresh browser. CI catches a broken image reference. |
| 5. Ship a browser game | Add a game catalog entry and one lazy-loaded game route. Keep game code in the client and instructions, credits, and cover art in content. | A direct game URL loads the game and its instructions. Visiting a poem does not fetch the game bundle. Returning to the gallery stops the game's animation and input handlers. |

## Kubernetes learning track

Start after milestone 2, as soon as the portable image passes its smoke tests. The production release can proceed independently.

1. Create a local kind cluster, load a commit-tagged image, and add a Deployment and ClusterIP Service under `deploy/kubernetes/`. Port-forward the Service and run the same HTTP checks as Compose. [kind quick start](https://kind.sigs.k8s.io/docs/user/quick-start/)
2. Add readiness and liveness probes against Go, set CPU and memory requests and limits, and scale to two replicas. Delete one pod, watch its replacement become ready, and verify the poem remains available through the Service.
3. Roll out a second image containing an edited poem. Observe rollout status and the served SHA. Try a release that fails readiness and practice undoing it. Finish when the earlier SHA and content are serving again. Record the commands that worked in the manifests directory.

## Later decisions

- **Move media to R2** when measured release size or build time becomes a burden. Pilot one gallery and verify its existing URLs still work after a rollback.
- **Add Minecraft hosting** when edition, audience, and hosting budget are known. Give the game its own hostname, persistent world, backups, and restore drill. Website deploys should be possible while players are online.

**First pull request:** deliver milestone 1 with one poem as the fixture for authoring, Go tests, and the browser reading experience. That URL becomes the smoke test for every deployment target.
