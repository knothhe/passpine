import snapshot from "../data/release.json";
import { parseRelease } from "./downloads";

/** A saved, validated release. The UI must not describe this as a live latest-release lookup. */
export const bundledRelease = parseRelease(snapshot);
