# Cloudflare release and rollback

Use a separate private R2 bucket and Worker in staging and production. Enable the account's `workers.dev` subdomain and Cloudflare Containers. Provision a GitHub Environment for each of `staging`, `production`, `staging-content`, and `production-content`.

| Environment | Variables | Secrets |
| --- | --- | --- |
| `staging`, `production` | `CLOUDFLARE_ACCOUNT_ID`, `WORKER_NAME`, `SITE_ORIGIN`, `R2_BUCKET`, `R2_ENDPOINT` | `CLOUDFLARE_API_TOKEN`, `R2_READ_ACCESS_KEY_ID`, `R2_READ_SECRET_ACCESS_KEY` |
| `staging-content`, `production-content` | `R2_BUCKET`, `R2_ENDPOINT` for the corresponding private bucket | `R2_WRITE_ACCESS_KEY_ID`, `R2_WRITE_SECRET_ACCESS_KEY` |

The deploy token needs permission to deploy Workers and Containers and read the bucket configuration. Give each S3 key access to one bucket. The publishing keys need Object Read & Write to upload and verify bytes. The keys passed to Go need Object Read. The Worker reads media using its private bucket binding. Set `SITE_ORIGIN` to the Worker's HTTPS URL and `R2_ENDPOINT` to the bucket's account or jurisdiction S3 endpoint.

## Deploy a release

Push to `main` for production or run the **Cloudflare release** workflow with **staging** and **deploy** on the commit to test. The workflow exports the writing snapshot, manifest, and media blobs from that checkout. The publisher conditionally creates each object and reads it back to verify its bytes. It uploads blobs before the manifest and snapshot. An interrupted or conflicting publish fails before the activation job. Rerun the failed jobs to resume the same release.

The activation job pushes the API image and uploads a Worker version tagged with the Git SHA. The version binds the frontend assets, R2 release SHA, and API image digest. It then shifts traffic to that version and verifies the public SHA, writing, HTML deep link, asset, and media bytes. The deployment summary records the Git SHA, version ID, and digest. The first deployment provisions the Durable Object container application and can take several minutes.

Keep prior Worker versions, registry image digests, and R2 objects used by recoverable releases. Disable any bucket lifecycle expiration for their keys. A release with the same Git SHA must always export identical bytes.

## Restore a release

Run **Cloudflare release** with the same target environment, **restore**, and the full Git SHA of a prior deployment. The workflow selects the retained Worker version by tag, shifts traffic to it, and repeats public verification against the restored commit. The version carries its own client assets and named API image, and the Worker selects the corresponding writing and media from R2. If the platform no longer retains a version, redeploy the corresponding Git commit with the same release SHA and retained content objects.

## Staging acceptance

1. Deploy commit A containing a poem and a web image. Update both files in commit B while keeping the poem slug and image filename. Deploy B to staging and record the verification results for both commits.
2. Restore A and confirm the verification step compares the original poem body, image bytes, and running SHA. Keep A active for the interruption drill.
3. Start a staging publish for a new revision with two changed images. Cancel during upload, before activation. Check A's version, writing, and media through its existing origin. Retry the same Git revision and confirm that the immutable uploads and public verification succeed.
4. Run `npm ci && npm run build && npm run test:worker` for the repeatable local fixture suite. In Linux, set `TEST_API_CONTAINER=1` to run Go in Docker. The suite checks empty, missing, and mismatched snapshots, retries after a partial upload, revalidation, and media while the Go API is stopped.

- [R2 S3 compatibility](https://developers.cloudflare.com/r2/api/s3/api/)
- [Container image pinning and retention](https://developers.cloudflare.com/containers/guides/image-management/)
- [Worker versions and deployments](https://developers.cloudflare.com/workers/versions-and-deployments/)
