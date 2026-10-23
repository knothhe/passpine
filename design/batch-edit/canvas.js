// Review boards only. Coordinates are CSS pixels at the intended desktop viewport.
globalThis.PASSPINE_BATCH_BOARDS = {
  width: 3640,
  height: 3590,
  boards: [
    { file: "browse.html", image: "browse.png", key: "sceneBrowse", x: 0, y: 90, w: 1180, h: 800 },
    { file: "empty.html", image: "empty.png", key: "sceneEmpty", x: 1230, y: 90, w: 1180, h: 800 },
    { file: "select.html", image: "select.png", key: "sceneSelect", x: 2460, y: 90, w: 1180, h: 800 },
    { file: "move.html", image: "move.png", key: "sceneMove", x: 0, y: 1000, w: 1180, h: 800 },
    { file: "conflict.html", image: "conflict.png", key: "sceneConflict", x: 1230, y: 1000, w: 1180, h: 800 },
    { file: "progress.html", image: "progress.png", key: "sceneProgress", x: 2460, y: 1000, w: 1180, h: 800 },
    { file: "trash.html", image: "trash.png", key: "sceneTrash", x: 0, y: 1940, w: 1180, h: 800 },
    { file: "success.html", image: "success.png", key: "sceneSuccess", x: 1230, y: 1940, w: 1180, h: 800 },
    { file: "partial.html", image: "partial.png", key: "scenePartial", x: 2460, y: 1940, w: 1180, h: 800 },
    { file: "select.html?theme=dark", image: "dark.png", key: "darkVariant", x: 0, y: 2840, w: 1180, h: 800 },
    { file: "move.html?lang=en", image: "english.png", key: "englishVariant", x: 1230, y: 2840, w: 1180, h: 800 },
    { file: "select.html", image: "narrow.png", key: "narrowVariant", x: 2460, y: 2840, w: 860, h: 760 },
  ],
  notes: [
    { key: "canvasSelectionNote", x: 0, y: 0, w: 1180 },
    { key: "canvasMoveNote", x: 0, y: 910, w: 1180 },
    { key: "canvasTrashNote", x: 0, y: 1850, w: 1180 },
    { key: "canvasVariantsNote", x: 0, y: 2750, w: 1180 },
  ],
};
