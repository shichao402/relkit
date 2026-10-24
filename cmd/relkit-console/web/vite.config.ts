import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

// The panel is served from the console binary at /-/admin/. Assets are emitted
// into the Go package that embeds them, so `go build` needs nothing else.
export default defineConfig({
  base: "/-/admin/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  build: {
    outDir: path.resolve(__dirname, "../internal/webui/dist"),
    emptyOutDir: true,
    assetsDir: "assets",
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      "/-/admin/api": {
        target: "http://127.0.0.1:8081",
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: "jsdom",
    // localStorage and cookies need a real origin; jsdom's default is opaque,
    // which makes the theme preference throw instead of returning null.
    environmentOptions: { jsdom: { url: "http://localhost/-/admin/" } },
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
