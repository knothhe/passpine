// Inserts each page's build-time markup into the documents Vite emitted.
import { readFile, writeFile } from "node:fs/promises";
import { createServer } from "vite";

const root = '<div id="root"></div>';
const server = await createServer({
  appType: "custom",
  logLevel: "error",
  server: { middlewareMode: true, hmr: false, ws: false },
});
try {
  const { render, pageDocuments } = await server.ssrLoadModule(
    "/src/prerender.tsx",
  );
  for (const [page, file] of Object.entries(pageDocuments)) {
    const path = new URL(`../dist/${file}`, import.meta.url);
    const html = await readFile(path, "utf8");
    if (!html.includes(root)) throw new Error(`${file} has no empty root`);
    await writeFile(path, html.replace(root, `<div id="root">${render(page)}</div>`));
  }
} finally {
  await server.close();
}
