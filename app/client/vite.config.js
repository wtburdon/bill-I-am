import { defineConfig } from "vite";

process.env.VITE_RELEASE_SHA ??= "dev";

export default defineConfig({
  server: {
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
});
