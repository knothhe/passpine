import { copyFile, mkdir } from "node:fs/promises";
import "./music.mjs";

const screenshots = new URL("../../../assets/website/", import.meta.url);
const demo = new URL("../public/demo/", import.meta.url);

for (const locale of ["zh-CN", "en"]) {
  await mkdir(new URL(`${locale}/`, demo), { recursive: true });
  for (const name of ["desktop.png", "extension.png"]) {
    const path = `${locale}/${name}`;
    await copyFile(new URL(path, screenshots), new URL(path, demo));
  }
}
