# Go backend

From `app/server`, validate the published content and start the service on port 8080.

```sh
go run . validate -content-dir ../content
go run . serve -content-dir ../content
```

Use `-addr` to change the listen address. Restart Go after editing content. Run Vite separately with `npm run dev` from the repository root.

## API image and local mounts

Build the API image from the repository root. Its runtime contains the Go binary and CA certificates. Mount a local content directory for offline development.

```sh
export GIT_SHA="$(git rev-parse HEAD)"
docker build --platform linux/amd64 --build-arg GIT_SHA="$GIT_SHA" -f app/server/Dockerfile -t billiam-api:local .
docker run --rm -p 127.0.0.1:8080:8080 \
  --mount "type=bind,source=$PWD/app/content,target=/content,readonly" \
  billiam-api:local serve -content-dir /content
docker compose -f deploy/compose.yaml up --build
```

For kind, point `$FIXTURE_FILE` at any valid writing `index.md`. Replace the ConfigMap and restart the Deployment when the fixture changes.

```sh
kind load docker-image billiam-api:local
kubectl create configmap billiam-writing --from-file=index.md="$FIXTURE_FILE"
kubectl apply -f deploy/kubernetes/api.yaml
kubectl port-forward service/billiam-api 8080:8080
```

## Published releases

Set `R2_RELEASE_SHA`, `R2_BUCKET`, `R2_ENDPOINT`, `R2_ACCESS_KEY_ID`, and `R2_SECRET_ACCESS_KEY` for production. Scope the credential to bucket reads. The configured release must match the SHA embedded in the image. Go loads the snapshot before opening its listener, with up to three S3 attempts and a 30-second startup deadline. It exits if it cannot load a valid snapshot. After startup, writing requests use the in-memory index.

From `app/server`, run `go run . export -content-dir ../content -release-dir "$RELEASE_DIR" -sha "$GIT_SHA"` to produce the snapshot and media manifest. See the [Cloudflare release runbook](../../deploy/cloudflare/README.md) for publishing and recovery.

Run `bash app/server/smoke-image.sh` from the repository root for the checked-out collection. Pass `renamed` or `empty` to run the other mounted fixtures.
