import { DurableObject } from "cloudflare:workers";
export { default } from "./router.js";

export class WritingAPI extends DurableObject {
  startup;
  ready = false;

  async readyPort(container) {
    if (!container.running) {
      container.start({
        image: container.images.api,
        enableInternet: true,
        env: {
          R2_RELEASE_SHA: this.env.RELEASE_SHA,
          R2_BUCKET: this.env.R2_BUCKET,
          R2_ENDPOINT: this.env.R2_ENDPOINT,
          R2_ACCESS_KEY_ID: this.env.R2_ACCESS_KEY_ID,
          R2_SECRET_ACCESS_KEY: this.env.R2_SECRET_ACCESS_KEY,
        },
      });
      await container.setInactivityTimeout(10 * 60 * 1000);
    }
    const port = container.getTcpPort(8080);
    const deadline = Date.now() + 60_000;
    while (Date.now() < deadline) {
      try {
        const probe = await port.fetch("http://container/readyz", {
          signal: AbortSignal.timeout(1000),
        });
        await probe.body?.cancel();
        if (probe.ok) return port;
      } catch {
        // A cold process has no listener until its snapshot is loaded.
      }
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    throw new Error("Writing API did not become ready");
  }

  async fetch(request) {
    const container = this.ctx.container;
    if (!container) throw new Error("Writing API container is unavailable");
    if (!container.running) this.ready = false;
    if (!this.ready || this.startup) {
      const startup = this.startup ??= this.readyPort(container);
      try {
        await startup;
        this.ready = true;
      } finally {
        if (this.startup === startup) this.startup = undefined;
      }
    }
    const target = new URL(request.url);
    target.protocol = "http:";
    target.host = "container";
    return await container.getTcpPort(8080).fetch(new Request(target, request));
  }
}
