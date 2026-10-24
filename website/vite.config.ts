import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";
import { zh } from "./src/constants/copy.ts";
import { pageDocuments, type PageId } from "./src/constants/routes.ts";

const pages = Object.entries(pageDocuments) as [PageId, string][];

export default defineConfig({
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  // Unknown paths get a real 404 locally, matching Cloudflare.
  appType: "mpa",
  build: {
    rolldownOptions: {
      input: Object.fromEntries(
        pages.map(([page, file]) => [
          page,
          fileURLToPath(new URL(file, import.meta.url)),
        ]),
      ),
    },
  },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "website-copy",
      transformIndexHtml: (html, { path }) => {
        const page = pages.find(([, file]) => path === `/${file}`)?.[0];
        if (!page) throw new Error(`No page metadata for ${path}`);
        const meta = zh.meta[page];
        return html
          .replaceAll("__TITLE__", meta.title)
          .replaceAll("__DESCRIPTION__", meta.description);
      },
    },
  ],
});
