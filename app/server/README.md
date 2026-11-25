# Go backend

From `app/server`, validate the published content and start the service on port 8080.

```sh
go run . validate -content-dir ../content
go run . serve -content-dir ../content
```

Use `-addr` to change the listen address and `-client-dir` to serve a Vite build. Restart the service after editing content. Running without a subcommand starts the server.

From the repository root, package the current content snapshot. Place the built frontend in `app/client/dist` before building an image that serves client routes.

```sh
docker build --platform linux/amd64 --build-arg GIT_SHA="$(git rev-parse HEAD)" -f app/server/Dockerfile -t bill-i-am .
docker run --rm -p 8080:8080 bill-i-am
```

Run `bash app/server/smoke-image.sh` from the repository to build and test the image with a temporary client fixture.
