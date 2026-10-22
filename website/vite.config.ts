import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";
import { zh } from "./src/constants/copy.ts";

export default defineConfig({
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "website-copy",
      transformIndexHtml: (html) =>
        html
          .replaceAll("__TITLE__", zh.meta.title)
          .replaceAll("__DESCRIPTION__", zh.meta.description),
    },
  ],
});
