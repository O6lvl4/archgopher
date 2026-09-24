import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// BASE is the path GitHub Pages serves the site under (/archgopher/).
export default defineConfig({
  base: process.env.BASE ?? "/",
  plugins: [react()],
  server: { port: 5176, fs: { allow: [".."] } },
  // Icons stay files so the browser fetches only the ones on screen instead of
  // every icon riding in the script.
  build: { assetsInlineLimit: (file) => (file.endsWith(".svg") ? false : undefined) },
});
