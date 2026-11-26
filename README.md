# bill-I-am

Billy's personal portfolio and creative site for writing, photography, drawings, games, and software projects.

Git is the authoring source. Go serves the writing API, Cloudflare R2 holds published content, and Workers Static Assets serves the React frontend. See the [platform architecture](docs/README.md), [release runbook](deploy/cloudflare/README.md), and [roadmap](docs/roadmap.md).

## Project structure

| Path | Purpose |
| --- | --- |
| app/client/ | React frontend |
| app/server/ | Go HTTP server |
| app/content/ | Writing, photo galleries, and drawings published through Git |
| deploy/ | Cloudflare, Docker Compose, and Kubernetes configuration |
| .github/workflows/ | GitHub Actions workflows |
| .agents/skills/ | Repository-specific agent skills |
| docs/ | Project documentation and architecture decisions |

## Local development

Start Go from the repository root, then run Vite in a second terminal.

```sh
(cd app/server && go run . serve -content-dir ../content)
```

```sh
npm ci
npm run dev
```

Vite proxies `/api` to Go on port 8080. Run `npm run build` to build the frontend. See the [Go API guide](app/server/README.md) for Docker, Compose, and Kubernetes.
