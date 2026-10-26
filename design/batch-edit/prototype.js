/* Isolated interaction study. No vault, network, clipboard, or product APIs. */
(() => {
  "use strict";
  const params = new URLSearchParams(location.search);
  const catalogs = globalThis.PASSPINE_BATCH_COPY;
  let language = params.get("lang") === "en" ? "en" : "zh-CN";
  let theme = params.get("theme") === "dark" ? "dark" : "light";
  const review = document.body.dataset.review === "true";
  const sceneKeys = {
    browse: "sceneBrowse", empty: "sceneEmpty", select: "sceneSelect", move: "sceneMove",
    conflict: "sceneConflict", trash: "sceneTrash", progress: "sceneProgress",
    strip: "sceneStrip", mixed: "sceneMixed", "trash-confirm": "sceneTrashConfirm", success: "sceneSuccess", partial: "scenePartial", "trash-success": "sceneTrash",
    "cleanup-success": "sceneCleanupSuccess", "undo-running": "sceneUndoRunning", "undo-partial": "sceneUndoPartial",
  };
  const seed = [
    { id: "cf", name: "Cloudflare", folder: "开发", username: "infra@example.test", website: "https://dash.cloudflare.com" },
    { id: "gh", name: "GitHub", folder: "开发", username: "pine@example.test", website: "https://github.com" },
    { id: "vc", name: "Vercel", folder: "开发", username: "web@example.test", website: "https://vercel.com" },
    { id: "fg", name: "Figma", folder: "工作", username: "design@example.test", website: "https://figma.com" },
    { id: "li", name: "Linear", folder: "工作", username: "team@example.test", website: "https://linear.app" },
    { id: "nt", name: "Notion 团队空间与长期项目资料", folder: "工作/共享账号", username: "docs@example.test", website: "https://notion.so" },
    { id: "gh2", name: "GitHub", folder: "工作/客户项目", username: "client@example.test", website: "https://github.com" },
    { id: "ap", name: "Apple", folder: "生活", username: "hello@example.test", website: "https://account.apple.com" },
    { id: "mail", name: "邮箱", folder: "生活", username: "pine@example.test", website: "https://mail.example.test" },
    { id: "nas", name: "NAS 家庭存储", folder: "家庭/设备", username: "admin", website: "https://nas.example.test" },
    { id: "oldli", name: "Linear", folder: "归档", username: "archive@example.test", website: "https://linear.app" },
    { id: "long", name: "家庭资料与长期订阅记录", folder: "生活/家庭资料", username: "family@example.test", website: "https://example.test" },
    { id: "st", name: "Stripe", folder: "工作/财务", username: "billing@example.test", website: "https://dashboard.stripe.com" },
    { id: "hr", name: "招聘平台", folder: "工作/招聘", username: "talent@example.test", website: "https://jobs.example.test" },
  ];
  const paths = {
    sidebar: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>',
    folder: '<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Z"/>',
    folderPlus: '<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Z"/><path d="M12 10v6m-3-3h6"/>',
    trash: '<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/>',
    search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    key: '<circle cx="8" cy="9" r="5"/><path d="m12 13 8 8M16 17l3-3M18 19l3-3"/>',
    close: '<path d="m6 6 12 12M6 18 18 6"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    minus: '<path d="M5 12h14"/>',
    arrow: '<path d="M4 12h16m-6-6 6 6-6 6"/>',
    back: '<path d="M20 12H4m6-6-6 6 6 6"/>',
    down: '<path d="m6 9 6 6 6-6"/>',
    copy: '<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
    eye: '<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>',
    edit: '<path d="m16 3 5 5-12 12-6 1 1-6Z"/><path d="m14 5 5 5"/>',
    lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
    draft: '<path d="M14 3H5v18h14V8Z"/><path d="M14 3v5h5M8 12h8M8 16h5"/>',
    settings: '<path d="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8Z"/><path d="m9 3-1 3-3 1-2 5 2 5 3 1 1 3h6l1-3 3-1 2-5-2-5-3-1-1-3Z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>',
    moon: '<path d="M20 15A9 9 0 0 1 9 4a9 9 0 1 0 11 11Z"/>',
    warning: '<path d="m12 3 10 18H2Z"/><path d="M12 9v5m0 3h.01"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/>',
    checkCircle: '<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>',
    external: '<path d="M14 3h7v7M21 3l-9 9M10 5H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5"/>',
    restore: '<path d="M3 10a9 9 0 1 1 1 8M3 4v6h6M12 7v5l3 2"/>',
    spinner: '<path d="M12 3a9 9 0 1 1-9 9"/>',
    cleanup: '<path d="m16 3 5 5-9 9-5-5Z"/><path d="m7 12-4 9h9l3-5M5 17l4 4M8 14l5 5"/>',
    chevron: '<path d="m9 6 6 6-6 6"/>',
    branch: '<circle cx="6" cy="18" r="2"/><circle cx="6" cy="6" r="2"/><circle cx="18" cy="6" r="2"/><path d="M6 8v8M18 8a6 6 0 0 1-6 6H8"/>',
    sync: '<path d="M20 11a8 8 0 0 0-14.3-4.9L4 8M4 4v4h4M4 13a8 8 0 0 0 14.3 4.9L20 16M20 20v-4h-4"/>',
    shield: '<path d="M12 3 5 6v5c0 4.5 3 8.3 7 10 4-1.7 7-5.5 7-10V6Z"/><path d="m9 12 2 2 4-4"/>',
    openLink: '<path d="M7 17 17 7M9 7h8v8"/>',
  };
  const esc = value => String(value ?? "").replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
  const t = (key, values = {}) => catalogs[language][key].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ""));
  const icon = name => `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.info}</svg>`;
  const path = item => item.folder ? `${item.folder}/${item.name}` : item.name;
  const folderLabel = folder => folder || t("rootFolder");
  const domain = website => { try { return new URL(website).hostname.replace(/^www\./, ""); } catch { return ""; } };
  const defaultIDs = ["cf", "gh", "fg", "li"];
  const root = document.getElementById("root");
  let items, trashItems, emptyFolders, s, timer, toastTimer, anchor = null;
  const button = (action, label, variant = "secondary", glyph = "", disabled = false, extra = "") => `<button class="button ${variant}" data-action="${action}" ${disabled ? "disabled" : ""} ${extra}>${glyph ? icon(glyph) : ""}<span>${esc(label)}</span></button>`;
  const iconButton = (action, key, glyph, disabled = false) => `<button class="icon-button" data-action="${action}" aria-label="${esc(t(key))}" title="${esc(t(key))}" ${disabled ? "disabled" : ""}>${icon(glyph)}</button>`;
  const byID = id => items.find(item => item.id === id);
  const selected = () => [...s.selected].map(byID).filter(Boolean);
  const collection = () => s.scope === "trash" ? trashItems : items.filter(item => s.scope === "__root__" ? item.folder === "" : s.scope === "all" || item.folder === s.scope || item.folder.startsWith(`${s.scope}/`));
  const visible = () => collection().filter(item => path(item).toLocaleLowerCase().includes(s.query.trim().toLocaleLowerCase()));
  const folders = () => [...new Set([...items.flatMap(item => item.folder.split("/").filter(Boolean).map((_, i, a) => a.slice(0, i + 1).join("/"))), ...emptyFolders])].sort();
  const busy = () => Boolean(s.progress);

  function init(scene, updateURL = true) {
    clearTimeout(timer); clearTimeout(toastTimer);
    items = seed.map(item => ({ ...item }));
    trashItems = [];
    emptyFolders = ["归档/旧项目", "临时", "临时/待整理"];
    s = { scene, mode: scene !== "browse", scope: "all", query: "", selected: new Set(), active: "gh", dialog: null,
      destination: null, folderQuery: "", keep: null, stripFrom: "", collapsed: new Set(folders()), result: null, progress: null, toast: null, undo: null, undoResult: null, sidebar: false, reveal: false };
    anchor = null;
    if (!["browse", "empty"].includes(scene)) s.selected = new Set(defaultIDs);
    if (scene === "move") { s.selected = new Set(["fg", "li"]); s.destination = "家庭/设备"; }
    if (scene === "strip") { s.selected = new Set(["fg", "li", "nt", "gh2"]); s.destination = "归档/旧项目"; }
    if (scene === "mixed") s.destination = "家庭/设备";
    if (scene === "conflict") { s.destination = "归档"; s.keep = "name"; s.selected.add("gh2"); }
    if (s.destination !== null) s.folderQuery = s.destination;
    if (scene === "trash-confirm") s.dialog = "trash-confirm";
    if (scene === "progress") { s.destination = "家庭/设备"; s.progress = { action: "move", ids: [...s.selected], done: 2, total: 4 }; }
    if (scene === "partial") applyPartial();
    if (scene === "success") applySuccess("move", defaultIDs, "工作/归档");
    if (["trash", "trash-success"].includes(scene)) applySuccess("trash", defaultIDs);
    if (scene === "cleanup-success") applyCleanup();
    if (["undo-running", "undo-partial"].includes(scene)) {
      applySuccess("move", defaultIDs, "工作/归档");
      if (scene === "undo-running") { s.progress = { action: "undo", total: 4, done: 2 }; s.toast = { key: "progressUndoing", values: { done: 2, total: 4 }, running: true }; }
      else {
        const records = s.undo.records;
        records.slice(0, 2).forEach(record => { byID(record.original.id).folder = record.original.folder; });
        items.push({ ...records[2].original, id: "undo-existing" });
        byID(records[3].original.id).username = "changed@example.test";
        s.undo.records = records.slice(2);
        s.undo.completed = 2;
        s.undoResult = { completed: 2, failed: [{ record: records[2], reason: "undoOriginalExists" }, { record: records[3], reason: "undoTargetChanged" }] };
        s.toast = null;
      }
    }
    if (updateURL && review) history.replaceState(null, "", `?${new URLSearchParams({ scene, lang: language, theme })}`);
    render();
    document.querySelector(".folder-option.selected")?.scrollIntoView({ block: "nearest" });
  }

  function applySuccess(action, ids, destination = "", changedPaths = null) {
    const originals = ids.map(byID).filter(Boolean).map(item => ({ ...item }));
    if (action === "move") ids.forEach(id => { const item = byID(id); if (item) item.folder = changedPaths ? changedPaths.get(id).split("/").slice(0, -1).join("/") : [destination, s.keep === "name" ? "" : item.folder].filter(Boolean).join("/"); });
    else { trashItems.push(...originals); items = items.filter(item => !ids.includes(item.id)); }
    s.mode = false; s.selected.clear(); s.active = null; s.dialog = null; s.progress = null; s.result = null;
    s.destination = null; s.folderQuery = "";
    s.undo = { action, completed: 0, records: originals.map(original => ({ original, appliedPath: action === "move" ? path(byID(original.id)) : path(original) })) };
    s.undoResult = null;
    s.toast = action === "move"
      ? { key: "movedSuccess", values: { count: originals.length, folder: folderLabel(destination) }, action: "undo", actionKey: "undo" }
      : { key: "trashedSuccess", values: { count: originals.length }, sub: "trashLocalNotice", action: "undo", actionKey: "undo" };
  }

  function applyPartial() {
    const originals = ["cf", "gh"].map(id => ({ ...byID(id) }));
    ["cf", "gh"].forEach(id => { byID(id).folder = "工作/归档"; s.selected.delete(id); });
    items.push({ ...byID("li"), id: "li-destination", folder: "工作/归档" });
    s.result = { kind: "partial", success: 2, destination: "工作/归档",
      failed: [{ id: "fg", reason: "writeBlocked", review: false }, { id: "li", reason: "destinationSaved", review: true }] };
    s.undo = { action: "move", completed: 0, records: originals.map(original => ({ original, appliedPath: path(byID(original.id)) })) };
    s.toast = { key: "partialCompleted", values: { count: 2 }, action: "undo", actionKey: "undo" };
  }

  function applyCleanup() {
    const removed = [...emptyFolders]; emptyFolders = [];
    s.mode = false; s.selected.clear(); s.result = null; s.undoResult = null;
    s.undo = { action: "cleanup", completed: 0, records: removed.map(folder => ({ folder })) };
    s.toast = { key: "cleanupSuccess", values: { count: removed.length }, sub: "cleanupUndoHelp", action: "undo", actionKey: "undo" };
  }

  function restoreUndoRecord(receipt, record) {
    if (receipt.action === "cleanup") {
      if (items.some(item => path(item) === record.folder)) return "undoOriginalExists";
      if (!emptyFolders.includes(record.folder)) emptyFolders.push(record.folder);
      return null;
    }
    const original = record.original;
    if (items.some(item => item.id !== original.id && path(item).toLocaleLowerCase() === path(original).toLocaleLowerCase())) return "undoOriginalExists";
    if (receipt.action === "trash") {
      const recovery = trashItems.find(item => item.id === original.id);
      if (!recovery) return "undoTargetChanged";
      items.push({ ...original }); trashItems = trashItems.filter(item => item.id !== original.id); return null;
    }
    const current = byID(original.id);
    if (!current || path(current) !== record.appliedPath || current.username !== original.username || current.website !== original.website) return "undoTargetChanged";
    current.folder = original.folder; return null;
  }

  function startUndo() {
    if (!s.undo || busy()) return;
    clearTimeout(toastTimer);
    const receipt = s.undo, records = [...receipt.records], failed = [];
    let completed = receipt.completed;
    s.undoResult = null; s.progress = { action: "undo", total: records.length, done: 0 };
    const step = () => {
      const record = records[s.progress.done];
      const reason = restoreUndoRecord(receipt, record);
      if (reason) failed.push({ record, reason });
      else {
        completed++;
        if (s.result) s.result.success = Math.max(0, s.result.success - 1);
      }
      s.progress.done++;
      s.toast = { key: "progressUndoing", values: { done: s.progress.done, total: records.length }, running: true }; render();
      if (s.progress.done < records.length) { timer = setTimeout(step, 400); return; }
      timer = setTimeout(() => {
        s.progress = null;
        receipt.completed = completed;
        if (failed.length) { receipt.records = failed.map(failure => failure.record); s.undoResult = { completed, failed }; s.toast = null; }
        else { s.undo = null; s.toast = { key: "undoSuccess", values: { count: completed } }; autoDismiss(); }
        render();
      }, 350);
    };
    s.toast = { key: "progressUndoing", values: { done: 0, total: records.length }, running: true }; render();
    timer = setTimeout(step, 400);
  }

  // Folders shared by every selected entry, shallowest first. Removing any of them
  // keeps the structure beneath it, so no selected entry can fail to match.
  function commonFolders() {
    const chosen = selected();
    if (!chosen.length || chosen.some(item => !item.folder)) return [];
    const split = chosen.map(item => item.folder.split("/")), shared = [];
    for (let i = 0; i < split[0].length && split.every(parts => parts[i] === split[0][i]); i++) shared.push(split[0][i]);
    return shared.map((_, i) => shared.slice(0, i + 1).join("/"));
  }

  // Until the user picks, removing the deepest shared folder is the default; without
  // a shared folder that option is unavailable and the full path is kept instead.
  const keepMode = () => {
    const mode = s.keep || "strip";
    return mode === "strip" && !commonFolders().length ? "full" : mode;
  };
  const keptPath = item => keepMode() === "name" ? item.name : keepMode() === "strip" ? path(item).slice(s.stripFrom.length + 1) : path(item);
  const targetPath = item => [s.destination, keptPath(item)].filter(Boolean).join("/");

  function syncStripSource() {
    if (busy()) return;
    const options = commonFolders();
    if (!options.includes(s.stripFrom)) s.stripFrom = options.at(-1) || "";
  }

  function movePlan() {
    const plan = { ready: [], skipped: [], status: new Map() };
    if (s.destination === null) return plan;
    const chosen = selected(), grouped = new Map();
    const key = item => targetPath(item).normalize("NFC").toLocaleLowerCase();
    chosen.forEach(item => grouped.set(key(item), (grouped.get(key(item)) || 0) + 1));
    chosen.forEach(item => {
      const target = key(item);
      const status = targetPath(item) === path(item) ? "unchanged"
        : items.some(other => other.id !== item.id && path(other).normalize("NFC").toLocaleLowerCase() === target) ? "exists"
        : grouped.get(target) > 1 ? "duplicate" : "ready";
      plan.status.set(item.id, status);
      (status === "ready" ? plan.ready : plan.skipped).push(item);
    });
    return plan;
  }

  function reviewBar() {
    if (!review) return "";
    return `<header class="review-bar"><div class="review-brand"><strong class="review-title">${t("reviewTitle")}</strong><span>${t("reviewNote")}</span></div>
      <nav class="review-nav" aria-label="${t("scenesLabel")}">${["browse", "select", "move", "strip", "mixed", "conflict", "progress", "success", "trash", "partial", "cleanup-success", "undo-running", "undo-partial"].map(scene => `<button data-scene="${scene}" class="${s.scene === scene ? "active" : ""}">${t(sceneKeys[scene])}</button>`).join("")}</nav>
      <div class="review-tools"><select id="language" aria-label="${t("language")}"><option value="zh-CN" ${language === "zh-CN" ? "selected" : ""}>${t("chineseLanguage")}</option><option value="en" ${language === "en" ? "selected" : ""}>${t("englishLanguage")}</option></select>${iconButton("theme", "appearance", theme === "dark" ? "sun" : "moon")}<button class="text-button" data-action="reset">${t("reset")}</button><a class="text-button" href="canvas.html?lang=${language}">${t("overview")}${icon("external")}</a></div></header>`;
  }

  // Each option previews the deepest selected path, where the three choices differ most.
  function keepField() {
    const options = commonFolders(), mode = keepMode();
    const sample = selected().reduce((longest, item) => !longest || path(item).length > path(longest).length ? item : longest, null);
    const base = s.destination === null ? "…/" : s.destination ? `${s.destination}/` : "";
    const source = options.length > 1
      ? `<select id="strip-from" aria-label="${t("keepStripFrom")}" ${busy() ? "disabled" : ""}>${options.map(folder => `<option value="${esc(folder)}" ${folder === s.stripFrom ? "selected" : ""}>${esc(folder)}</option>`).join("")}</select>`
      : options.length ? `<code class="keep-source">${esc(options[0])}</code>` : "";
    const option = (value, label, kept, extra = "", unavailable = "") => `<label class="keep-option ${unavailable ? "disabled" : ""}"><input type="radio" id="keep-${value}" name="keep" value="${value}" ${mode === value ? "checked" : ""} ${unavailable || busy() ? "disabled" : ""}><span class="keep-body"><span class="keep-title"><span>${esc(label)}</span>${extra}</span>${unavailable ? `<span class="keep-hint">${esc(unavailable)}</span>` : sample ? `<span class="keep-example"><span class="keep-example-base">${esc(base)}</span>${esc(kept)}</span>` : ""}</span></label>`;
    return `<fieldset class="keep-field" ${busy() ? "disabled" : ""}><legend class="section-label">${t("keepLabel")}</legend>
      ${option("strip", t("keepStrip"), options.length && sample ? path(sample).slice(s.stripFrom.length + 1) : "", source, options.length ? "" : t("keepStripUnavailable"))}
      ${option("full", t("keepFull"), sample ? path(sample) : "")}
      ${option("name", t("keepName"), sample?.name || "")}</fieldset>`;
  }

  // Skipped entries come first, so their reasons stay visible without scrolling.
  function pathPreview(plan) {
    const heading = `<h4 class="section-label" id="path-preview-title">${t("pathPreview")}</h4>`;
    if (s.destination === null) return `<section class="path-preview-section" aria-labelledby="path-preview-title">${heading}<p class="muted preview-empty">${t("previewEmpty")}</p></section>`;
    const reasons = { exists: "previewExists", duplicate: "previewDuplicate", unchanged: "previewUnchanged" };
    const rows = [...plan.skipped, ...plan.ready].map(item => {
      const status = plan.status.get(item.id);
      return `<li class="preview-row ${status}"><span class="preview-from">${esc(path(item))}</span><span class="preview-to">${icon("arrow")}<span class="visually-hidden">${t("previewTo")}</span><span>${esc(targetPath(item))}</span></span>${status === "ready" ? "" : `<span class="preview-reason">${icon(status === "unchanged" ? "info" : "warning")}<span>${t(reasons[status])}</span></span>`}</li>`;
    }).join("");
    const summary = plan.skipped.length ? t("previewSummary", { ready: plan.ready.length, skipped: plan.skipped.length }) : t("previewSummaryAll", { count: plan.ready.length });
    const notice = (glyph, key) => `<p class="inline-notice">${icon(glyph)}<span>${t(key)}</span></p>`;
    return `<section class="path-preview-section" aria-labelledby="path-preview-title">${heading}<p class="preview-summary" role="status">${summary}</p>
      ${!plan.ready.length ? notice("info", "noMovable") : ""}${s.destination.startsWith("工作/共享账号") ? notice("info", "policyNotice") : ""}
      <ul class="preview-list" aria-label="${t("previewListLabel")}">${rows}</ul></section>`;
  }

  function navigation() {
    const navItem = (scope, label, glyph, count = "") => `<button class="nav-item ${s.scope === scope ? "active" : ""}" data-scope="${esc(scope)}" ${busy() ? "disabled" : ""} ${s.scope === scope ? 'aria-current="page"' : ""}>${icon(glyph)}<span class="nav-label">${esc(label)}</span>${count !== "" ? `<span class="nav-count">${count}</span>` : ""}</button>`;
    const topFolders = folders().filter(folder => !folder.includes("/"));
    return `<aside class="sidebar ${s.sidebar ? "is-open" : ""}" aria-label="${t("navigation")}"><div class="sidebar-main"><p class="sidebar-section-label">${t("passwordLibrary")}</p>${navItem("all", t("allEntries"), "key", items.length)}<p class="sidebar-section-label">${t("folders")}</p>${items.some(item => !item.folder) ? navItem("__root__", t("rootFolder"), "folder") : ""}${topFolders.map(folder => navItem(folder, folder, "folder")).join("")}</div><div class="sidebar-bottom">${navItem("drafts", t("drafts"), "draft")}${navItem("trash", t("trash"), "trash", trashItems.length || "")}<button class="nav-item" data-action="demo">${icon("settings")}<span class="nav-label">${t("settings")}</span></button></div></aside>`;
  }

  function checkbox(state) {
    return `<span class="check-slot" aria-hidden="true"><span class="entry-check ${state}">${state === "checked" ? icon("check") : state === "mixed" ? icon("minus") : ""}</span></span>`;
  }

  function list() {
    const rows = visible();
    const title = s.scope === "all" ? t("allEntries") : s.scope === "trash" ? t("trash") : s.scope === "__root__" ? t("rootFolder") : s.scope;
    const all = rows.length > 0 && rows.every(item => s.selected.has(item.id));
    const some = rows.some(item => s.selected.has(item.id));
    return `<section class="library" aria-labelledby="list-title"><header class="library-heading"><div class="library-title"><h1 id="list-title">${esc(title)}</h1><p class="library-count" aria-label="${t("entryCountLabel")}">${t(s.query ? "resultCount" : "entryCount", { count: rows.length, total: collection().length })}</p></div><div class="heading-buttons">${s.scope !== "trash" ? `${iconButton("cleanup", "cleanupFolders", "cleanup", busy() || s.mode)}<button class="text-button" data-action="mode" ${busy() ? "disabled" : ""}>${t(s.mode ? "cancel" : "select")}</button>${s.mode ? "" : iconButton("demo", "newEntry", "plus")}` : ""}</div></header>
      <div class="search-wrap">${icon("search")}<input id="search" aria-label="${t("searchLabel")}" placeholder="${t("search")}" value="${esc(s.query)}" ${busy() ? "disabled" : ""}>${s.query ? iconButton("clear-search", "clearSearch", "close") : ""}</div>
      ${s.mode ? `<div class="selection-toolbar"><button class="text-button select-all" data-action="all" role="checkbox" aria-checked="${all ? "true" : some ? "mixed" : "false"}" title="${t("selectAllHint")}" ${busy() || !rows.length ? "disabled" : ""}>${checkbox(all ? "checked" : some ? "mixed" : "")}<span>${t("selectAll")}</span></button></div>` : ""}
      <div class="entry-list ${s.mode ? "selecting" : ""}" id="entry-list" tabindex="0" aria-label="${esc(title)}">${rows.length ? rows.map(item => {
        const checked = s.mode && s.selected.has(item.id), active = !s.mode && item.id === s.active;
        return `<button class="entry-row ${checked ? "checked" : ""} ${active ? "selected" : ""}" data-entry="${item.id}" ${s.mode ? `role="checkbox" aria-checked="${checked}"` : `aria-pressed="${active}"`} aria-label="${esc(t(s.mode ? "selectEntry" : "openEntry", { path: path(item) }))}" ${busy() ? "disabled" : ""}>
          ${s.mode ? checkbox(checked ? "checked" : "") : ""}<span class="entry-text"><span class="entry-name">${esc(item.name)}</span><span class="entry-path" title="${esc(path(item))}">${esc(active ? item.username : path(item))}</span></span>${active ? `<span class="row-chevron">${icon("chevron")}</span>` : s.mode || s.scope === "trash" ? "" : `<span class="row-copy" data-copy="${item.id}" role="presentation">${icon("copy")}</span>`}</button>`;
      }).join("") : `<div class="empty-state">${icon("search")}<strong>${t(s.scope === "trash" && !s.query ? "trashEmpty" : "emptyResults")}</strong><p>${t(s.scope === "trash" && !s.query ? "trashHelp" : "emptyResultsDescription")}</p></div>`}</div></section>`;
  }

  function batchPanel() {
    const count = s.selected.size, p = s.progress;
    if (!count && !p) return `<div class="batch-panel"><div class="panel-toolbar"></div><div class="batch-heading"><h2>${t("chooseItems")}</h2><p class="batch-subtitle">${t("chooseItemsDescription")}</p></div></div>`;
    const running = action => p?.action === action;
    const progress = action => running(action) ? 'aria-busy="true"' : "";
    const plan = movePlan();
    const folderOptions = ["", ...folders()];
    const query = s.folderQuery.trim().toLocaleLowerCase();
    const candidate = s.folderQuery.trim().replace(/^\/+|\/+$/g, "");
    const canCreate = candidate && !folderOptions.includes(candidate) && !candidate.split("/").some(segment => [".", ".."].includes(segment));
    // Searching flattens the tree to full paths, since a deep match needs its parents to be told apart.
    const shown = query ? folderOptions.filter(folder => folderLabel(folder).toLocaleLowerCase().includes(query)) : folderOptions;
    const moveLabel = running("move") ? t("progressMoving", { done: p.done, total: p.total }) : t("moveCount", { count: s.destination === null ? count : plan.ready.length });
    const trashLabel = running("trash") ? t("progressTrashing", { done: p.done, total: p.total }) : t("trashAction");
    // One move form covers moving, flattening and replacing a shared prefix; trash is a separate action.
    return `<div class="batch-panel selection-panel"><div class="panel-toolbar"></div>
      <div class="selection-heading"><h2>${t("selectedCount", { count: p ? p.total : count })}</h2>${button("trash", trashLabel, "quiet danger-text", running("trash") ? "spinner" : "trash", busy(), progress("trash"))}</div>
      <section class="move-section" aria-label="${t("moveSection")}">
        <h4 class="section-label" id="destination-title">${t("destinationLabel")}</h4>
        <div class="folder-search search-wrap">${icon("search")}<input id="folder-search" placeholder="${t("folderSearch")}" aria-label="${t("folderSearchLabel")}" value="${esc(s.folderQuery)}" ${busy() ? "disabled" : ""}></div>
        <div class="folder-list ${query ? "flat" : ""}" role="group" aria-labelledby="destination-title">${query ? shown.map(folder => folderOption(folder, folderLabel(folder), 0, false)).join("") : folderTree()}${canCreate ? `<button class="folder-option folder-create" data-folder="${esc(candidate)}">${icon("folderPlus")}<span>${esc(t("createFolder", { folder: candidate }))}</span></button>` : ""}${!shown.length && !canCreate ? `<p class="muted">${t("noFolders")}</p>` : ""}</div>
        ${keepField()}
        ${pathPreview(plan)}
        <footer class="move-footer">${button("confirm-move", moveLabel, "primary", running("move") ? "spinner" : "", busy() || !plan.ready.length, progress("move"))}</footer>
      </section></div>`;
  }

  function detailPanel() {
    const item = s.scope === "trash" ? trashItems.find(entry => entry.id === s.active) : byID(s.active);
    if (!item) return `<div class="empty-state"><div class="empty-symbol">${icon(s.scope === "trash" ? "trash" : "shield")}</div><h2>${t(s.scope === "trash" ? "trashEmpty" : "noDetail")}</h2><p>${t(s.scope === "trash" ? "trashHelp" : "noDetailDescription")}</p>${s.scope === "trash" ? "" : `<button class="text-button empty-action" data-action="demo">${t("newEntry")}</button>`}</div>`;
    const crumbs = `<nav class="breadcrumb" aria-label="${t("entryPath")}">${icon("folder")}${(item.folder ? item.folder.split("/") : [t("rootFolder")]).map(part => `<span>${esc(part)}</span>${icon("chevron")}`).join("")}<span class="current">${esc(item.name)}</span></nav>`;
    if (s.scope === "trash") return `<div class="batch-panel"><div class="panel-toolbar">${crumbs}</div><div class="entry-title"><h2>${esc(item.name)}</h2><p>${t("trashHelp")}</p></div><div class="batch-actions">${button("restore", t("restore"), "primary", "restore")}</div></div>`;
    const copyButton = key => `<button class="copy-button" data-action="copy" aria-label="${t(key)}">${icon("copy")}<span>${t("copy")}</span></button>`;
    return `<div class="batch-panel"><div class="panel-toolbar">${crumbs}<div class="detail-actions">${iconButton("reveal", "showAllFields", "eye")}${iconButton("demo", "editEntry", "edit")}${iconButton("trash-one", "trashAction", "trash")}</div></div>
      <div class="entry-title"><h2>${esc(item.name)}</h2><p>${esc(domain(item.website))}</p></div>
      <div class="field-row"><div><p class="field-label">${t("username")}</p><span class="field-value">${esc(item.username)}</span></div>${copyButton("copyUsername")}</div>
      <div class="field-row"><div><p class="field-label">${t("password")}</p><span class="field-value secret-value">${s.reveal ? "Example-only-2026" : "••••••••••••"}</span></div>${iconButton("reveal", s.reveal ? "hidePassword" : "showPassword", "eye")}${copyButton("copyPassword")}</div>
      <div class="field-row"><div><p class="field-label">${t("website")}</p><span class="field-value">${esc(item.website)}</span></div>${iconButton("demo", "openWebsite", "openLink")}${copyButton("copyWebsite")}</div>
      <div class="detail-notes"><p class="field-label">${t("notes")}</p><p>${t("demoNotes")}</p></div>
      <p class="file-row"><span>${t("encryptedFile")}</span><code>${esc(path(item))}.gpg</code></p></div>`;
  }

  function resultPanel() {
    const r = s.result;
    const retryable = r.failed.some(failure => !failure.review);
    return `<div class="batch-panel"><div class="panel-toolbar"></div><div class="result-summary">${icon("warning")}<h2>${t("partialTitle", { success: r.success, failed: r.failed.length })}</h2><p class="result-description">${t("partialUndoHelp")}</p></div><div class="result-list">${r.failed.map(failure => {
      const item = byID(failure.id);
      return `<div class="result-row"><strong class="entry-name">${esc(item.name)}</strong><span class="entry-path">${esc(path(item))}</span><p class="result-description">${t(failure.reason)}</p>${failure.review ? `<button class="text-button" data-action="check-both">${t("checkBoth")}${icon("arrow")}</button>` : ""}</div>`;
    }).join("")}</div><div class="batch-actions">${retryable ? button("retry", t("retryFailed"), "primary", "restore", busy()) : ""}${button("result-close", t("finish"), retryable ? "secondary" : "primary", "", busy())}</div></div>`;
  }

  function undoResultPanel() {
    const r = s.undoResult;
    return `<div class="batch-panel"><div class="panel-toolbar"></div><div class="result-summary">${icon("warning")}<h2>${t("undoPartialTitle", { success: r.completed, failed: r.failed.length })}</h2><p class="result-description">${t("undoPartialHelp")}</p></div><div class="result-list">${r.failed.map(failure => `<div class="result-row"><strong class="entry-name">${esc(failure.record.original?.name || failure.record.folder)}</strong><span class="entry-path">${esc(failure.record.original ? path(failure.record.original) : failure.record.folder)}</span><p class="result-description">${t(failure.reason)}</p></div>`).join("")}</div>${s.result ? `<p class="inline-notice">${t("forwardFailuresKept", { count: s.result.failed.length })}</p>` : ""}<div class="batch-actions">${button("retry-undo", t("retryUndo"), "primary", "restore", busy())}${button("undo-result-close", t(s.result ? "backToResult" : "finish"), "secondary", "", busy())}</div></div>`;
  }

  function folderOption(folder, label, depth, collapsible) {
    const chosen = s.destination === folder, open = !s.collapsed.has(folder);
    const toggle = collapsible
      ? `<button class="folder-toggle" data-toggle-folder="${esc(folder)}" aria-expanded="${open}" aria-label="${esc(t(open ? "collapseFolder" : "expandFolder", { folder }))}">${icon("chevron")}</button>`
      : `<span class="folder-toggle" aria-hidden="true"></span>`;
    return `<div class="folder-node" style="--depth:${depth}">${toggle}<button class="folder-option ${chosen ? "selected" : ""}" data-folder="${esc(folder)}" aria-pressed="${chosen}" ${busy() ? "disabled" : ""} aria-label="${esc(t("chooseFolder", { folder: folderLabel(folder) }))}">${icon("folder")}<span class="folder-option-path">${esc(label)}</span>${chosen ? icon("check") : ""}</button></div>`;
  }

  function folderTree() {
    const all = folders();
    const childCount = folder => all.filter(other => other.startsWith(`${folder}/`) && !other.slice(folder.length + 1).includes("/")).length;
    const hidden = folder => [...s.collapsed].some(parent => folder.startsWith(`${parent}/`));
    return [folderOption("", t("rootFolder"), 0, false), ...all.filter(folder => !hidden(folder)).map(folder => {
      const parts = folder.split("/");
      return folderOption(folder, parts.at(-1), parts.length - 1, childCount(folder) > 0);
    })].join("");
  }

  function modal() {
    if (s.dialog === "trash-confirm") return `<dialog class="modal" aria-labelledby="modal-title" aria-describedby="modal-description"><header class="modal-header"><h2 id="modal-title">${t("trashConfirmTitle", { count: s.selected.size })}</h2></header><div class="modal-body"><p id="modal-description">${t("trashConfirmDescription")}</p></div><footer class="modal-footer">${button("cancel-dialog", t("cancel"), "secondary")}${button("confirm-trash", t("trashAction"), "danger", "trash")}</footer></dialog>`;
    if (s.dialog !== "check") return "";
    return `<dialog class="modal" aria-labelledby="modal-title"><header class="modal-header"><h2 id="modal-title">${t("checkBothTitle")}</h2></header><div class="modal-body"><div class="path-preview"><p class="field-label">${t("originalPath")}</p><p>${esc(path(byID("li")))}</p><p class="field-label">${t("destinationPath")}</p><p>${esc(`${s.result.destination}/${byID("li").name}`)}</p></div></div><footer class="modal-footer">${button("cancel-dialog", t("back"), "primary")}</footer></dialog>`;
  }

  function toastView() {
    if (!s.toast) return "";
    const { key, values, sub, action, actionKey, running } = s.toast;
    return `<div class="toast-region" role="status"><div class="toast ${running ? "undo-running" : ""}">${icon(running ? "spinner" : "checkCircle")}<span class="toast-copy"><span>${esc(t(key, values))}</span>${sub ? `<span class="toast-sub">${t(sub)}</span>` : ""}</span>${action ? `<button class="text-button toast-action" data-action="${action}" ${busy() ? "disabled" : ""}>${t(actionKey)}</button>` : ""}${running ? "" : iconButton("close-toast", "close", "close")}</div></div>`;
  }

  function render() {
    syncStripSource();
    const previous = document.activeElement;
    const focusID = previous?.id;
    const range = previous instanceof HTMLInputElement ? [previous.selectionStart, previous.selectionEnd] : null;
    const listScroll = document.getElementById("entry-list")?.scrollTop || 0;
    const inspectorScroll = document.querySelector(".inspector-scroll")?.scrollTop || 0;
    const folderScroll = document.querySelector(".folder-list")?.scrollTop || 0;
    const wasDialog = Boolean(document.querySelector("dialog[open]"));
    document.documentElement.dataset.theme = theme;
    document.documentElement.lang = language;
    document.title = t("pageTitle");
    root.innerHTML = `${reviewBar()}<section class="app-window" aria-label="${t("brand")}"><header class="titlebar"><div class="titlebar-left"><div class="titlebar-controls">${iconButton("demo", "previous", "back", true)}${iconButton("demo", "next", "arrow", true)}<button class="icon-button" data-action="sidebar" aria-label="${t("toggleSidebar")}" title="${t("toggleSidebar")}">${icon("sidebar")}</button></div><strong class="app-title">${t("passwordLibrary")}</strong></div><div class="titlebar-right"><button class="vault-switcher" data-action="demo" aria-label="${t("switchVault")}">${icon("folder")}<span>${t("personalVault")}</span>${icon("down")}</button><button class="status-button" data-action="demo">${icon("branch")}<span>${t("gitNotInitialized")}</span></button><button class="status-button" data-action="demo" aria-label="${t("syncVault")}">${icon("sync")}<span>${t("sync")}</span></button>${iconButton("demo", "lock", "lock")}</div></header><div class="workspace">${navigation()}${s.sidebar ? `<button class="sidebar-backdrop" data-action="sidebar" aria-label="${t("toggleSidebar")}"></button>` : ""}<div class="main-card">${list()}<section class="inspector" aria-live="polite"><div class="inspector-scroll">${s.undoResult ? undoResultPanel() : s.result ? resultPanel() : s.mode ? batchPanel() : detailPanel()}</div></section></div></div>${toastView()}</section>${modal()}`;
    const modalNode = document.querySelector("dialog");
    if (modalNode) {
      modalNode.showModal();
      modalNode.addEventListener("cancel", event => { event.preventDefault(); closeDialog(); });
      modalNode.addEventListener("click", event => { if (event.target === modalNode) { const rect = modalNode.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) closeDialog(); } });
    }
    document.getElementById("entry-list").scrollTop = listScroll;
    document.querySelector(".inspector-scroll").scrollTop = inspectorScroll;
    const folderList = document.querySelector(".folder-list"); if (folderList) folderList.scrollTop = folderScroll;
    bind();
    if (focusID && document.getElementById(focusID) && (!modalNode || wasDialog)) {
      const next = document.getElementById(focusID); next.focus({ preventScroll: true });
      if (range && next instanceof HTMLInputElement) next.setSelectionRange(...range);
    } else if (modalNode && !wasDialog) modalNode.querySelector("button")?.focus({ preventScroll: true });
  }

  function toast(key, values = {}) {
    s.toast = { key, values }; render(); autoDismiss();
  }

  function autoDismiss(ms = 5000) {
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { s.toast = null; s.undo = null; render(); }, ms);
  }

  function closeDialog() { const action = s.dialog === "trash-confirm" ? "trash" : "check-both"; s.dialog = null; render(); document.querySelector(`[data-action="${action}"]`)?.focus({ preventScroll: true }); }

  function toggleEntry(id, event) {
    if (busy()) return;
    if (s.scope === "trash") { s.active = id; s.result = null; render(); return; }
    if (event.metaKey || event.ctrlKey || event.shiftKey) s.mode = true;
    s.result = null;
    if (!s.mode) s.active = id;
    else if (event.shiftKey && anchor) {
      const ids = visible().map(item => item.id), a = ids.indexOf(anchor), b = ids.indexOf(id);
      ids.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(key => s.selected.add(key));
    } else { s.selected.has(id) ? s.selected.delete(id) : s.selected.add(id); anchor = id; }
    render(); document.querySelector(`[data-entry="${id}"]`)?.focus({ preventScroll: true });
  }

  function startOperation(action) {
    const plan = action === "move" ? movePlan() : null;
    const ids = action === "move" ? plan.ready.map(item => item.id) : [...s.selected];
    if (!ids.length || busy()) return;
    const destination = s.destination || "";
    const retained = plan ? plan.skipped.map(item => item.id) : [];
    s.dialog = null; s.progress = { action, ids, total: ids.length, done: 0 }; s.toast = null; render();
    const step = () => {
      if (!s.progress) return;
      s.progress.done++; render();
      if (s.progress.done < ids.length) timer = setTimeout(step, 320);
      else timer = setTimeout(() => {
        const changedPaths = new Map(ids.map(id => [id, targetPath(byID(id))]));
        applySuccess(action, ids, destination, action === "move" ? changedPaths : null);
        // Entries excluded from the operation stay selected for another action.
        if (retained.length) { s.mode = true; s.selected = new Set(retained); }
        render(); autoDismiss(8000);
      }, 300);
    };
    timer = setTimeout(step, 350);
  }

  function handle(action) {
    if (busy() && !["theme", "sidebar"].includes(action)) return;
    switch (action) {
      case "theme": theme = theme === "dark" ? "light" : "dark"; render(); break;
      case "reset": init("select"); break;
      case "sidebar": s.sidebar = !s.sidebar; render(); break;
      case "mode": s.mode = !s.mode; s.selected.clear(); s.result = null; s.destination = null; s.folderQuery = ""; render(); break;
      case "all": {
        const ids = visible().map(item => item.id), all = ids.every(id => s.selected.has(id));
        s.mode = true; s.result = null; ids.forEach(id => all ? s.selected.delete(id) : s.selected.add(id)); render(); break;
      }
      case "clear-search": s.query = ""; render(); document.getElementById("search")?.focus(); break;
      case "trash": if (s.selected.size) { s.dialog = "trash-confirm"; render(); } break;
      case "trash-one": s.mode = true; s.selected = new Set([s.active]); s.dialog = "trash-confirm"; render(); break;
      case "confirm-trash": startOperation("trash"); break;
      case "cancel-dialog": closeDialog(); break;
      case "confirm-move": startOperation("move"); break;
      case "cleanup": if (!s.mode && emptyFolders.length) { applyCleanup(); render(); autoDismiss(8000); } else toast("cleanupNone"); break;
      case "reveal": s.reveal = !s.reveal; render(); break;
      case "undo": case "retry-undo": startUndo(); break;
      case "undo-result-close": s.undoResult = null; render(); break;
      case "result-close": s.result = null; s.mode = false; s.selected.clear(); s.active = null; render(); break;
      case "check-both": s.dialog = "check"; render(); break;
      case "retry": {
        const r = s.result;
        const retry = r.failed.filter(failure => !failure.review);
        const originals = retry.map(failure => ({ ...byID(failure.id) }));
        retry.forEach(failure => { byID(failure.id).folder = r.destination; s.selected.delete(failure.id); });
        r.success += retry.length; r.failed = r.failed.filter(failure => failure.review);
        const records = originals.map(original => ({ original, appliedPath: path(byID(original.id)) }));
        s.undo = { action: "move", completed: s.undo?.action === "move" ? s.undo.completed : 0, records: [...(s.undo?.action === "move" ? s.undo.records : []), ...records] };
        s.toast = { key: "partialCompleted", values: { count: s.undo.records.length }, action: "undo", actionKey: "undo" }; render(); autoDismiss(8000); break;
      }
      case "restore": {
        const item = trashItems.find(entry => entry.id === s.active);
        if (item && !items.some(existing => path(existing) === path(item))) { items.push(item); trashItems = trashItems.filter(entry => entry.id !== item.id); s.active = trashItems[0]?.id; toast("restored", { count: 1 }); }
        break;
      }
      case "close-toast": clearTimeout(toastTimer); s.toast = null; s.undo = null; render(); break;
      case "copy": toast("copiedDemo"); break;
      default: toast("previewOnly");
    }
  }

  function bind() {
    root.querySelectorAll("[data-scene]").forEach(node => node.addEventListener("click", () => init(node.dataset.scene)));
    root.querySelectorAll("[data-action]").forEach(node => node.addEventListener("click", () => handle(node.dataset.action)));
    root.querySelectorAll("[data-scope]").forEach(node => node.addEventListener("click", () => {
      if (busy()) return;
      if (node.dataset.scope === "drafts") { toast("previewOnly"); return; }
      s.scope = node.dataset.scope; s.query = ""; s.selected.clear(); s.result = null; anchor = null; s.active = null; s.sidebar = false;
      if (s.scope === "trash") { s.mode = false; s.active = trashItems[0]?.id; }
      render(); document.getElementById("entry-list").scrollTop = 0;
    }));
    root.querySelectorAll("[data-entry]").forEach(node => node.addEventListener("click", event => {
      if (event.target.closest("[data-copy]")) { toast("copiedDemo"); return; }
      toggleEntry(node.dataset.entry, event);
    }));
    // Searching only filters the list; the selection survives it.
    document.getElementById("search")?.addEventListener("input", event => { s.query = event.target.value; s.result = null; render(); });
    document.getElementById("folder-search")?.addEventListener("input", event => { s.folderQuery = event.target.value; s.destination = folders().includes(s.folderQuery.trim()) ? s.folderQuery.trim() : null; render(); });
    document.querySelectorAll("[data-toggle-folder]").forEach(node => node.addEventListener("click", () => {
      const folder = node.dataset.toggleFolder;
      s.collapsed.has(folder) ? s.collapsed.delete(folder) : s.collapsed.add(folder); render();
      document.querySelector(`[data-toggle-folder="${CSS.escape(folder)}"]`)?.focus({ preventScroll: true });
    }));
    document.querySelectorAll("[data-folder]").forEach(node => node.addEventListener("click", () => {
      s.destination = node.dataset.folder; s.folderQuery = s.destination; render();
      document.querySelector(`[data-folder="${CSS.escape(s.destination)}"]`)?.focus({ preventScroll: true });
    }));
    // Choosing which prefix to remove also chooses that option; it only changes the draft.
    document.getElementById("strip-from")?.addEventListener("change", event => { s.stripFrom = event.target.value; s.keep = "strip"; render(); });
    document.querySelectorAll('input[name="keep"]').forEach(node => node.addEventListener("change", () => { s.keep = node.value; render(); }));
    document.getElementById("language")?.addEventListener("change", event => { language = event.target.value; render(); });
  }

  document.addEventListener("keydown", event => {
    if (event.key === "Escape") {
      if (s.dialog) return; // The native dialog cancel event handles the top layer.
      if (s.sidebar) { s.sidebar = false; render(); }
      else if (s.mode && !busy()) { s.mode = false; s.selected.clear(); s.result = null; render(); }
    }
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "a" && s.scope !== "trash" && !s.dialog && !busy() && !event.target.closest("input,textarea,select")) {
      event.preventDefault(); s.mode = true; s.result = null; visible().forEach(item => s.selected.add(item.id)); render(); document.getElementById("entry-list").focus({ preventScroll: true });
    }
    if (["ArrowDown", "ArrowUp"].includes(event.key) && event.target.closest(".entry-list") && !busy()) {
      const rows = [...document.querySelectorAll("[data-entry]")], index = rows.indexOf(document.activeElement);
      event.preventDefault(); rows[Math.max(0, Math.min(rows.length - 1, index + (event.key === "ArrowDown" ? 1 : -1)))]?.focus();
    }
  });
  const initial = params.get("scene") || document.body.dataset.scene || "select";
  init(Object.hasOwn(sceneKeys, initial) ? initial : "select", false);
})();
