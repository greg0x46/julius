import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// A self-contained resource, built from the same components as the website.
export default defineConfig({
  plugins: [react(), {
    name: "mcp-app-html",
    generateBundle(_options, bundle) {
      const entry = Object.values(bundle).find((item) => item.type === "chunk" && item.isEntry);
      if (!entry || entry.type !== "chunk") throw new Error("Missing MCP App entry");
      this.emitFile({
        type: "asset",
        fileName: "mcp-investments.html",
        source: `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Carteira de investimentos</title></head><body><div id="root"></div><script>${entry.code.replace(/<\/script/gi, "<\\/script")}</script></body></html>`,
      });
      delete bundle[entry.fileName];
    },
  }],
  define: { "process.env.NODE_ENV": JSON.stringify("production") },
  build: {
    emptyOutDir: false,
    lib: { entry: "src/mcp/main.tsx", name: "JuliusPortfolio", formats: ["iife"] },
  },
});
