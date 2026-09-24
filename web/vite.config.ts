import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// BASE is the path GitHub Pages serves the site under (/archgopher/).
export default defineConfig({
  base: process.env.BASE ?? "/",
  plugins: [react()],
  server: { port: 5176, fs: { allow: [".."] } },
});
