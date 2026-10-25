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
    prefix: "scenePrefix", "prefix-excluded": "scenePrefixExcluded", "prefix-no-folder": "scenePrefixNoFolder", "trash-confirm": "sceneTrashConfirm", success: "sceneSuccess", partial: "scenePartial", "trash-success": "sceneTrash",
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
  let items, trashItems, s, timer, toastTimer, anchor = null;
  const button = (action, label, variant = "secondary", glyph = "", disabled = false, extra = "") => `<button class="button ${variant}" data-action="${action}" ${disabled ? "disabled" : ""} ${extra}>${glyph ? icon(glyph) : ""}<span>${esc(label)}</span></button>`;
  const iconButton = (action, key, glyph, disabled = false) => `<button class="icon-button" data-action="${action}" aria-label="${esc(t(key))}" title="${esc(t(key))}" ${disabled ? "disabled" : ""}>${icon(glyph)}</button>`;
  const byID = id => items.find(item => item.id === id);
  const selected = () => [...s.selected].map(byID).filter(Boolean);
  const collection = () => s.scope === "trash" ? trashItems : items.filter(item => s.scope === "__root__" ? item.folder === "" : s.scope === "all" || item.folder === s.scope || item.folder.startsWith(`${s.scope}/`));
  const visible = () => collection().filter(item => path(item).toLocaleLowerCase().includes(s.query.trim().toLocaleLowerCase()));
  const folders = () => [...new Set(items.flatMap(item => item.folder.split("/").filter(Boolean).map((_, i, a) => a.slice(0, i + 1).join("/"))))].sort();
  const busy = () => Boolean(s.progress);
  // Entry names are user data; only the separator and the overflow template are copy.
  const nameList = entries => {
    const names = [...new Set(entries.map(item => item.name))];
    if (names.length <= 3) return names.join(t("nameSeparator"));
    return t("moreNames", { names: names.slice(0, 2).join(t("nameSeparator")), count: names.length - 2 });
  };

  function init(scene, updateURL = true) {
    clearTimeout(timer); clearTimeout(toastTimer);
    items = seed.map(item => ({ ...item }));
    trashItems = [];
    s = { scene, mode: scene !== "browse", scope: "all", query: "", selected: new Set(), active: "gh", dialog: null,
      destination: null, folderQuery: "", operation: "folder", prefixInitialized: false, prefixFrom: "", prefixTo: "", collapsed: new Set(folders()), result: null, progress: null, toast: null, undo: null, sidebar: false, reveal: false };
    anchor = null;
    if (!["browse", "empty"].includes(scene)) s.selected = new Set(defaultIDs);
    if (["move", "conflict"].includes(scene)) s.destination = scene === "conflict" ? "归档" : "家庭/设备";
    if (s.destination !== null) s.folderQuery = s.destination;
    if (["prefix", "prefix-excluded", "prefix-no-folder"].includes(scene)) { s.operation = "prefix"; s.prefixInitialized = true; s.selected = new Set(["fg", "li"]); s.prefixFrom = "工作"; s.prefixTo = "工作0"; }
    if (scene === "prefix-excluded") { s.selected = new Set(["fg", "gh2", "li", "cf"]); items.push({ ...byID("fg"), id: "prefix-existing", folder: "工作0" }); }
    if (scene === "prefix-no-folder") { selected().forEach(item => { item.folder = ""; }); s.prefixFrom = ""; s.prefixTo = ""; }
    if (scene === "trash-confirm") s.dialog = "trash-confirm";
    if (scene === "conflict") s.selected.add("gh2");
    if (scene === "progress") { s.destination = "家庭/设备"; s.progress = { action: "move", ids: [...s.selected], done: 2, total: 4 }; }
    if (scene === "partial") applyPartial();
    if (scene === "success") applySuccess("move", defaultIDs, "工作/归档");
    if (["trash", "trash-success"].includes(scene)) applySuccess("trash", defaultIDs);
    if (updateURL && review) history.replaceState(null, "", `?${new URLSearchParams({ scene, lang: language, theme })}`);
    render();
    document.querySelector(".folder-option.selected")?.scrollIntoView({ block: "nearest" });
  }

  function applySuccess(action, ids, destination = "") {
    const originals = ids.map(byID).filter(Boolean).map(item => ({ ...item }));
    if (action === "move") ids.forEach(id => { if (byID(id)) byID(id).folder = destination; });
    else { trashItems.push(...originals); items = items.filter(item => !ids.includes(item.id)); }
    s.mode = false; s.selected.clear(); s.active = null; s.dialog = null; s.progress = null; s.result = null;
    s.lastDestination = destination; s.destination = null; s.folderQuery = "";
    s.undo = action === "trash" ? originals : null;
    s.toast = action === "move"
      ? { key: "movedSuccess", values: { count: originals.length, folder: folderLabel(destination) }, action: "destination", actionKey: "view" }
      : { key: "trashedSuccess", values: { count: originals.length }, sub: "trashLocalNotice", action: "undo", actionKey: "undo" };
  }

  function applyPartial() {
    ["cf", "gh"].forEach(id => { byID(id).folder = "工作/归档"; s.selected.delete(id); });
    items.push({ ...byID("li"), id: "li-destination", folder: "工作/归档" });
    s.result = { kind: "partial", success: 2, destination: "工作/归档",
      failed: [{ id: "fg", reason: "writeBlocked", review: false }, { id: "li", reason: "destinationSaved", review: true }] };
  }

  const targetPath = item => s.operation === "prefix"
    ? s.prefixFrom && (item.folder === s.prefixFrom || item.folder.startsWith(`${s.prefixFrom}/`))
      ? [s.prefixTo.trim(), path(item).slice(s.prefixFrom.length + 1)].filter(Boolean).join("/") : path(item)
    : [s.destination, item.name].filter(Boolean).join("/");

  const prefixOptions = () => [...new Set(selected().flatMap(item => {
    if (!item.folder) return [];
    const parts = item.folder.split("/");
    return [item.folder, ...parts.slice(0, -1).map((_, index) => parts.slice(0, index + 1).join("/"))];
  }))];

  function setPrefixSource(next) {
    const previous = s.prefixFrom;
    if (!s.prefixInitialized || s.prefixTo !== "" && s.prefixTo === previous) s.prefixTo = next;
    s.prefixFrom = next; s.prefixInitialized = true;
  }

  function syncPrefixSource() {
    if (s.operation !== "prefix" || busy()) return;
    const options = prefixOptions();
    if (!s.prefixInitialized || !options.includes(s.prefixFrom)) setPrefixSource(selected()[0]?.folder || options[0] || "");
  }

  const prefixDestinationValid = () => s.prefixTo.trim() !== "" && s.prefixTo.split("/").every(segment => segment && segment !== "." && segment !== "..") && !s.prefixTo.includes("\\");

  function movePlan() {
    const chosen = selected();
    if (s.operation !== "prefix" && s.destination === null) return { ready: [], conflicts: [], unchanged: [] };
    if (s.operation === "prefix" && !s.prefixFrom) return { ready: [], conflicts: [], unchanged: chosen };
    if (s.operation === "prefix" && !prefixDestinationValid()) return { ready: [], conflicts: [], unchanged: chosen };
    const key = item => targetPath(item).normalize("NFC").toLocaleLowerCase();
    const grouped = new Map();
    chosen.forEach(item => grouped.set(key(item), (grouped.get(key(item)) || 0) + 1));
    const plan = { ready: [], conflicts: [], unchanged: [] };
    chosen.forEach(item => {
      if (targetPath(item) === path(item)) { plan.unchanged.push(item); return; }
      const target = key(item);
      const exists = items.some(other => other.id !== item.id && path(other).normalize("NFC").toLocaleLowerCase() === target);
      (grouped.get(key(item)) > 1 || exists ? plan.conflicts : plan.ready).push(item);
    });
    return plan;
  }

  function reviewBar() {
    if (!review) return "";
    return `<header class="review-bar"><div class="review-brand"><strong class="review-title">${t("reviewTitle")}</strong><span>${t("reviewNote")}</span></div>
      <nav class="review-nav" aria-label="${t("scenesLabel")}">${["browse", "select", "move", "prefix", "conflict", "progress", "success", "trash", "partial"].map(scene => `<button data-scene="${scene}" class="${s.scene === scene ? "active" : ""}">${t(sceneKeys[scene])}</button>`).join("")}</nav>
      <div class="review-tools"><select id="language" aria-label="${t("language")}"><option value="zh-CN" ${language === "zh-CN" ? "selected" : ""}>${t("chineseLanguage")}</option><option value="en" ${language === "en" ? "selected" : ""}>${t("englishLanguage")}</option></select>${iconButton("theme", "appearance", theme === "dark" ? "sun" : "moon")}<button class="text-button" data-action="reset">${t("reset")}</button><a class="text-button" href="canvas.html?lang=${language}">${t("overview")}${icon("external")}</a></div></header>`;
  }

  function prefixSummary(plan) {
    const excluded = selected().filter(item => !plan.ready.includes(item));
    const reason = item => !s.prefixFrom ? "prefixNoSourceReason" : !prefixDestinationValid() ? "prefixInvalidDestinationReason" : plan.conflicts.includes(item) ? "prefixConflictReason"
      : item.folder !== s.prefixFrom && !item.folder.startsWith(`${s.prefixFrom}/`) ? "prefixMismatchReason" : "prefixUnchangedReason";
    return `<section class="prefix-summary" aria-labelledby="prefix-summary-title"><h3 class="section-label" id="prefix-summary-title">${t("pathPreview")}</h3><p class="prefix-summary-count" role="status">${t("prefixSummary", { ready: plan.ready.length, skipped: excluded.length })}</p>${excluded.length ? `<div class="prefix-excluded-list" role="group" aria-label="${t("prefixExcluded")}">${excluded.map(item => `<div class="prefix-excluded-row"><span>${esc(path(item))}</span><p>${t(reason(item))}</p></div>`).join("")}</div>` : `<p class="muted prefix-all-ready">${t("prefixAllReady")}</p>`}</section>`;
  }

  function prefixSourceField() {
    const options = prefixOptions();
    const value = options.length > 1
      ? `<select id="prefix-from" aria-label="${t("prefixFrom")}" ${busy() ? "disabled" : ""}>${options.map(folder => `<option value="${esc(folder)}" ${folder === s.prefixFrom ? "selected" : ""}>${esc(folder)}</option>`).join("")}</select>`
      : `<span class="prefix-source-value ${s.prefixFrom ? "" : "muted"}" aria-label="${t("prefixFrom")}">${esc(s.prefixFrom || t("prefixNoSource"))}</span>`;
    return `<div class="prefix-field"><span class="prefix-label">${t("prefixFrom")}</span><div class="prefix-source">${value}${button("copy-prefix", t("copy"), "quiet prefix-copy", "copy", busy() || !s.prefixFrom, `id="copy-prefix" aria-label="${t("copyPrefix")}" title="${t("copyPrefix")}"`)}</div>${!s.prefixFrom ? `<p class="prefix-source-hint">${t("prefixChooseSource")}</p>` : ""}</div>`;
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
    return `<section class="library" aria-labelledby="list-title"><header class="library-heading"><div class="library-title"><h1 id="list-title">${esc(title)}</h1><p class="library-count" aria-label="${t("entryCountLabel")}">${t(s.query ? "resultCount" : "entryCount", { count: rows.length, total: collection().length })}</p></div><div class="heading-buttons">${s.scope !== "trash" ? `<button class="text-button" data-action="mode" ${busy() ? "disabled" : ""}>${t(s.mode ? "cancel" : "select")}</button>${s.mode ? "" : iconButton("demo", "newEntry", "plus")}` : ""}</div></header>
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
    const moveTotal = s.operation === "prefix" || s.destination !== null ? plan.ready.length : count;
    const moveLabel = running("move") ? t(s.operation === "prefix" ? "progressUpdating" : "progressMoving", { done: p.done, total: p.total }) : t(s.operation === "prefix" ? "updateCount" : "moveCount", { count: moveTotal });
    const trashLabel = running("trash") ? t("progressTrashing", { done: p.done, total: p.total }) : t("trashAction");
    const operationTitle = t(s.operation === "prefix" ? "replacePrefix" : "moveSection");
    const operationHeading = `<header class="batch-operation-heading"><h3 id="batch-operation-title">${operationTitle}</h3>${button("confirm-move", moveLabel, "primary", running("move") ? "spinner" : "", busy() || !plan.ready.length, progress("move"))}</header>`;
    return `<div class="batch-panel selection-panel"><div class="panel-toolbar"></div>
      <div class="selection-heading"><h2>${t("selectedCount", { count: p ? p.total : count })}</h2></div>
      <div class="batch-actions organization-actions" role="group" aria-label="${t("batchActions")}">${button("folder-mode", t("moveSection"), "quiet", "folder", busy(), `id="batch-folder-mode" aria-pressed="${s.operation === "folder"}"`)}${button("prefix-mode", t("replacePrefix"), "quiet", "edit", busy(), `id="batch-prefix-mode" aria-pressed="${s.operation === "prefix"}"`)}${button("trash", trashLabel, "quiet danger-text", running("trash") ? "spinner" : "trash", busy(), progress("trash"))}</div>
      ${s.operation === "prefix" ? `<section class="move-section prefix-section" aria-labelledby="batch-operation-title">${operationHeading}<p class="muted">${t("prefixHelp")}</p>${prefixSourceField()}<label>${t("prefixTo")}<input id="prefix-to" value="${esc(s.prefixTo)}" placeholder="${t("prefixToPlaceholder")}" aria-label="${t("prefixTo")}" ${busy() ? "disabled" : ""}>${!s.prefixTo.trim() ? `<span class="prefix-input-hint">${t("prefixToRequired")}</span>` : ""}</label>${prefixSummary(plan)}</section>` : `<section class="move-section" aria-labelledby="batch-operation-title">${operationHeading}
        <div class="folder-search search-wrap">${icon("search")}<input id="folder-search" placeholder="${t("folderSearch")}" aria-label="${t("folderSearchLabel")}" value="${esc(s.folderQuery)}" ${busy() ? "disabled" : ""}></div>
        <div class="folder-list ${query ? "flat" : ""}" role="group" aria-label="${t("chooseDestination")}">${query ? shown.map(folder => folderOption(folder, folderLabel(folder), 0, false)).join("") : folderTree()}${canCreate ? `<button class="folder-option folder-create" data-folder="${esc(candidate)}">${icon("folderPlus")}<span>${esc(t("createFolder", { folder: candidate }))}</span></button>` : ""}${!shown.length && !canCreate ? `<p class="muted">${t("noFolders")}</p>` : ""}</div>
        ${moveNotices(plan)}
      </section>`}</div>`;
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
    return `<div class="batch-panel"><div class="panel-toolbar"></div><div class="result-summary">${icon("warning")}<h2>${t("partialTitle", { success: r.success, failed: r.failed.length })}</h2></div><div class="result-list">${r.failed.map(failure => {
      const item = byID(failure.id);
      return `<div class="result-row"><strong class="entry-name">${esc(item.name)}</strong><span class="entry-path">${esc(path(item))}</span><p class="result-description">${t(failure.reason)}</p>${failure.review ? `<button class="text-button" data-action="check-both">${t("checkBoth")}${icon("arrow")}</button>` : ""}</div>`;
    }).join("")}</div><div class="batch-actions">${retryable ? button("retry", t("retryFailed"), "primary", "restore") : ""}${button("result-close", t("finish"), retryable ? "secondary" : "primary")}</div></div>`;
  }

  function moveNotices(plan) {
    if (s.destination === null) return "";
    const notice = (kind, glyph, text) => `<p class="inline-notice ${kind}">${icon(glyph)}<span>${esc(text)}</span></p>`;
    return [
      plan.conflicts.length ? notice("warning", "warning", t("moveConflict", { names: nameList(plan.conflicts), folder: folderLabel(s.destination) })) : "",
      plan.unchanged.length ? notice("", "info", t("unchangedNotice", { count: plan.unchanged.length })) : "",
      !plan.ready.length ? notice("", "info", t("noMovable")) : "",
      s.destination.startsWith("工作/共享账号") ? notice("", "info", t("policyNotice")) : "",
    ].join("");
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
    const { key, values, sub, action, actionKey } = s.toast;
    return `<div class="toast-region" role="status"><div class="toast">${icon("checkCircle")}<span class="toast-copy"><span>${esc(t(key, values))}</span>${sub ? `<span class="toast-sub">${t(sub)}</span>` : ""}</span>${action ? `<button class="text-button toast-action" data-action="${action}">${t(actionKey)}</button>` : ""}${iconButton("close-toast", "close", "close")}</div></div>`;
  }

  function render() {
    syncPrefixSource();
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
    root.innerHTML = `${reviewBar()}<section class="app-window" aria-label="${t("brand")}"><header class="titlebar"><div class="titlebar-left"><div class="titlebar-controls">${iconButton("demo", "previous", "back", true)}${iconButton("demo", "next", "arrow", true)}<button class="icon-button" data-action="sidebar" aria-label="${t("toggleSidebar")}" title="${t("toggleSidebar")}">${icon("sidebar")}</button></div><strong class="app-title">${t("passwordLibrary")}</strong></div><div class="titlebar-right"><button class="vault-switcher" data-action="demo" aria-label="${t("switchVault")}">${icon("folder")}<span>${t("personalVault")}</span>${icon("down")}</button><button class="status-button" data-action="demo">${icon("branch")}<span>${t("gitNotInitialized")}</span></button><button class="status-button" data-action="demo" aria-label="${t("syncVault")}">${icon("sync")}<span>${t("sync")}</span></button>${iconButton("demo", "lock", "lock")}</div></header><div class="workspace">${navigation()}${s.sidebar ? `<button class="sidebar-backdrop" data-action="sidebar" aria-label="${t("toggleSidebar")}"></button>` : ""}<div class="main-card">${list()}<section class="inspector" aria-live="polite"><div class="inspector-scroll">${s.result ? resultPanel() : s.mode ? batchPanel() : detailPanel()}</div></section></div></div>${toastView()}</section>${modal()}`;
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
    const retained = plan ? [...plan.conflicts, ...(s.operation === "prefix" ? plan.unchanged : [])].map(item => item.id) : [];
    s.dialog = null; s.progress = { action, ids, total: ids.length, done: 0 }; s.toast = null; render();
    const step = () => {
      if (!s.progress) return;
      s.progress.done++; render();
      if (s.progress.done < ids.length) timer = setTimeout(step, 320);
      else timer = setTimeout(() => {
        const changedPaths = new Map(ids.map(id => [id, targetPath(byID(id))]));
        const prefix = action === "move" && s.operation === "prefix";
        applySuccess(action, ids, destination);
        if (prefix) { for (const [id, next] of changedPaths) byID(id).folder = next.split("/").slice(0, -1).join("/"); s.toast = { key: "prefixSuccess", values: { count: ids.length } }; }
        // Entries excluded from the operation stay selected for another action.
        if (retained.length) { s.mode = true; s.selected = new Set(retained); }
        render(); autoDismiss(action === "trash" ? 8000 : 5000);
      }, 300);
    };
    timer = setTimeout(step, 350);
  }

  function handle(action) {
    if (busy() && !["theme", "sidebar"].includes(action)) return;
    switch (action) {
      case "folder-mode": s.operation = "folder"; render(); break;
      case "prefix-mode": {
        s.operation = "prefix";
        render(); break;
      }
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
      case "reveal": s.reveal = !s.reveal; render(); break;
      case "undo": {
        if (!s.undo) break;
        const ids = s.undo.map(item => item.id);
        trashItems = trashItems.filter(item => !ids.includes(item.id));
        items.push(...s.undo); s.undo = null;
        s.mode = true; s.selected = new Set(ids);
        toast("undone"); break;
      }
      case "destination": {
        const destination = s.lastDestination ?? "";
        s.scope = destination || "__root__"; s.query = ""; s.mode = false; s.result = null; s.toast = null; s.selected.clear(); s.active = items.find(item => item.folder === destination)?.id; render(); break;
      }
      case "result-close": s.result = null; s.mode = false; s.selected.clear(); s.active = null; render(); break;
      case "check-both": s.dialog = "check"; render(); break;
      case "retry": {
        const r = s.result;
        const retry = r.failed.filter(failure => !failure.review);
        retry.forEach(failure => { byID(failure.id).folder = r.destination; s.selected.delete(failure.id); });
        r.success += retry.length; r.failed = r.failed.filter(failure => failure.review);
        toast("movedSuccess", { count: retry.length, folder: folderLabel(r.destination) }); break;
      }
      case "restore": {
        const item = trashItems.find(entry => entry.id === s.active);
        if (item && !items.some(existing => path(existing) === path(item))) { items.push(item); trashItems = trashItems.filter(entry => entry.id !== item.id); s.active = trashItems[0]?.id; toast("restored", { count: 1 }); }
        break;
      }
      case "close-toast": clearTimeout(toastTimer); s.toast = null; s.undo = null; render(); break;
      case "copy": toast("copiedDemo"); break;
      case "copy-prefix": toast("prefixCopiedDemo"); break;
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
    document.getElementById("prefix-from")?.addEventListener("change", event => { setPrefixSource(event.target.value); render(); });
    document.getElementById("prefix-to")?.addEventListener("input", event => { s.prefixTo = event.target.value; render(); });
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
