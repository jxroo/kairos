import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  base: "/dashboard/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/health": "http://127.0.0.1:7777",
      "/memories": "http://127.0.0.1:7777",
      "/conversations": "http://127.0.0.1:7777",
      "/v1": "http://127.0.0.1:7777",
      "/index": "http://127.0.0.1:7777",
      "/config": "http://127.0.0.1:7777",
      "/documents": "http://127.0.0.1:7777",
      "/search": "http://127.0.0.1:7777",
      "/tools": "http://127.0.0.1:7777",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
