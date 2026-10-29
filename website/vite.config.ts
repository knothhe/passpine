import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { readdir, readFile } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import type { Plugin } from "vite";
import { zh } from "./src/constants/copy.ts";
import { pageDocuments, type PageId } from "./src/constants/routes.ts";

const pages = Object.entries(pageDocuments) as [PageId, string][];

// Product screenshots are shared repository assets. Serve and emit them under
// /images so the prerendered HTML and the client bundle use the same URLs.
const imagesRoot = fileURLToPath(new URL("../assets/website", import.meta.url));
async function imageFiles(directory = imagesRoot): Promise<string[]> {
  const entries = await readdir(directory, { withFileTypes: true });
  const nested = await Promise.all(
    entries.map((entry) =>
      entry.isDirectory()
        ? imageFiles(join(directory, entry.name))
        : [join(directory, entry.name)],
    ),
  );
  return nested.flat().filter((file) => file.endsWith(".png"));
}
function websiteImages(): Plugin {
  return {
    name: "website-images",
    configureServer(server) {
      server.middlewares.use(async (request, response, next) => {
        const path = request.url?.split("?")[0] ?? "";
        if (!path.startsWith("/images/")) return next();
        const file = join(imagesRoot, decodeURIComponent(path.slice(8)));
        if (relative(imagesRoot, file).startsWith("..")) return next();
        try {
          const data = await readFile(file);
          response.setHeader("Content-Type", "image/png");
          response.end(data);
        } catch {
          next();
        }
      });
    },
    async generateBundle() {
      for (const file of await imageFiles()) {
        this.emitFile({
          type: "asset",
          fileName: `images/${relative(imagesRoot, file).split(sep).join("/")}`,
          source: await readFile(file),
        });
      }
    },
  };
}

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
    websiteImages(),
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
