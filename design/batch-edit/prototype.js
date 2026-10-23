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
    success: "sceneSuccess", partial: "scenePartial", "trash-success": "sceneTrashSuccess",
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
    chevron: '<path d="m9 5 7 7-7 7"/>',
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
    selection: '<rect x="3" y="3" width="7" height="7" rx="1.5"/><path d="m15 7 2 2 4-5M3 15h7M3 20h7M15 15h6M15 20h6"/>',
    external: '<path d="M14 3h7v7M21 3l-9 9M10 5H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-5"/>',
    restore: '<path d="M3 10a9 9 0 1 1 1 8M3 4v6h6M12 7v5l3 2"/>',
  };
  const esc = value => String(value ?? "").replace(/[&<>"']/g, ch => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));
  const t = (key, values = {}) => catalogs[language][key].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ""));
  const icon = name => `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.info}</svg>`;
  const path = item => item.folder ? `${item.folder}/${item.name}` : item.name;
  const folderLabel = folder => folder || t("rootFolder");
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

  function init(scene, updateURL = true) {
    clearTimeout(timer); clearTimeout(toastTimer);
    items = seed.map(item => ({ ...item }));
    trashItems = [];
    s = { scene, mode: scene !== "browse", scope: "all", query: "", selected: new Set(), active: "gh", dialog: null,
      destination: null, folderQuery: "", folderPickerOpen: true, result: null, progress: null, toast: null, sidebar: false, reveal: false };
    anchor = null;
    if (!["browse", "empty"].includes(scene)) s.selected = new Set(defaultIDs);
    if (["move", "conflict"].includes(scene)) { s.dialog = "move"; s.destination = scene === "conflict" ? "归档" : "家庭/设备"; s.folderPickerOpen = false; }
    if (scene === "conflict") s.selected.add("gh2");
    if (scene === "trash") s.dialog = "trash";
    if (s.dialog) s.returnAction = s.dialog;
    if (scene === "progress") s.progress = { action: "move", ids: [...s.selected], done: 2, total: 4, frozen: true };
    if (scene === "partial") applyPartial();
    if (scene === "success") applySuccess("move", defaultIDs, "工作/归档");
    if (scene === "trash-success") applySuccess("trash", defaultIDs);
    if (updateURL && review) history.replaceState(null, "", `?${new URLSearchParams({ scene, lang: language, theme })}`);
    render();
  }

  function applySuccess(action, ids, destination = "") {
    const originals = ids.map(byID).filter(Boolean).map(item => ({ ...item }));
    if (action === "move") ids.forEach(id => { if (byID(id)) byID(id).folder = destination; });
    else { trashItems.push(...originals); items = items.filter(item => !ids.includes(item.id)); }
    s.mode = false; s.selected.clear(); s.active = null; s.dialog = null; s.progress = null;
    s.result = { kind: action === "move" ? "moved" : "trashed", count: originals.length, destination, originals };
    s.toast = { key: action === "move" ? "movedSuccess" : "trashedSuccess", values: { count: originals.length }, action: action === "move" ? "destination" : "view-trash" };
  }

  function applyPartial() {
    const originals = defaultIDs.map(id => ({ ...byID(id) }));
    ["cf", "gh"].forEach(id => { byID(id).folder = "工作/归档"; s.selected.delete(id); });
    items.push({ ...byID("li"), id: "li-destination", folder: "工作/归档" });
    s.result = { kind: "partial", success: 2, destination: "工作/归档", originals,
      failed: [{ id: "fg", reason: "writeBlocked", review: false }, { id: "li", reason: "destinationSaved", review: true }] };
  }

  function movePlan() {
    const chosen = selected();
    if (s.destination === null) return { ready: [], conflicts: [], unchanged: [] };
    const grouped = new Map();
    chosen.forEach(item => { const key = `${s.destination}/${item.name}`.normalize("NFC").toLocaleLowerCase(); grouped.set(key, [...(grouped.get(key) || []), item]); });
    const plan = { ready: [], conflicts: [], unchanged: [] };
    chosen.forEach(item => {
      if (item.folder === s.destination) { plan.unchanged.push(item); return; }
      const target = s.destination ? `${s.destination}/${item.name}` : item.name;
      const internal = grouped.get(`${s.destination}/${item.name}`.normalize("NFC").toLocaleLowerCase()).length > 1;
      const exists = items.some(other => other.id !== item.id && path(other).normalize("NFC").toLocaleLowerCase() === target.normalize("NFC").toLocaleLowerCase());
      if (internal || exists) plan.conflicts.push({ ...item, reason: internal ? "selectedConflict" : "existingConflict" });
      else plan.ready.push(item);
    });
    return plan;
  }

  function reviewBar() {
    if (!review) return "";
    return `<header class="review-bar"><div class="review-brand"><strong class="review-title">${t("reviewTitle")}</strong><span>${t("reviewNote")}</span></div>
      <nav class="review-nav" aria-label="${t("scenesLabel")}">${["browse", "select", "move", "conflict", "trash", "partial"].map(scene => `<button data-scene="${scene}" class="${s.scene === scene ? "active" : ""}">${t(sceneKeys[scene])}</button>`).join("")}</nav>
      <div class="review-tools"><select id="language" aria-label="${t("language")}"><option value="zh-CN" ${language === "zh-CN" ? "selected" : ""}>${t("chineseLanguage")}</option><option value="en" ${language === "en" ? "selected" : ""}>${t("englishLanguage")}</option></select>${iconButton("theme", "appearance", theme === "dark" ? "sun" : "moon")}<button class="text-button" data-action="reset">${t("reset")}</button><a class="text-button" href="canvas.html?lang=${language}">${t("overview")}${icon("external")}</a></div></header>`;
  }

  function navigation() {
    const navItem = (scope, label, glyph, count = "") => `<button class="nav-item ${s.scope === scope ? "active" : ""}" data-scope="${esc(scope)}" ${busy() ? "disabled" : ""} ${s.scope === scope ? 'aria-current="page"' : ""}>${icon(glyph)}<span class="nav-label">${esc(label)}</span>${count !== "" ? `<span class="nav-count">${count}</span>` : ""}</button>`;
    const topFolders = folders().filter(folder => !folder.includes("/"));
    return `<aside class="sidebar ${s.sidebar ? "is-open" : ""}" aria-label="${t("navigation")}"><div class="sidebar-main"><p class="sidebar-section-label">${t("passwordLibrary")}</p>${navItem("all", t("allEntries"), "key", items.length)}<p class="sidebar-section-label">${t("folders")}</p>${items.some(item => !item.folder) ? navItem("__root__", t("rootFolder"), "folder") : ""}${topFolders.map(folder => navItem(folder, folder, "folder")).join("")}</div><div class="sidebar-bottom">${navItem("drafts", t("drafts"), "draft")}${navItem("trash", t("trash"), "trash", trashItems.length || "")}<button class="nav-item" data-action="demo">${icon("settings")}<span class="nav-label">${t("settings")}</span></button></div></aside>`;
  }

  function list() {
    const rows = visible();
    const title = s.scope === "all" ? t("allEntries") : s.scope === "trash" ? t("trash") : s.scope === "__root__" ? t("rootFolder") : s.scope;
    const all = rows.length > 0 && rows.every(item => s.selected.has(item.id));
    return `<section class="library" aria-labelledby="list-title"><header class="library-heading"><h1 id="list-title">${esc(title)}</h1><div class="heading-buttons">${s.scope !== "trash" ? `<button class="text-button" data-action="mode" ${busy() ? "disabled" : ""}>${t(s.mode ? "finish" : "select")}</button>${s.mode ? "" : iconButton("demo", "newEntry", "plus")}` : ""}</div></header>
      <div class="search-wrap">${icon("search")}<input id="search" aria-label="${t("searchLabel")}" placeholder="${t("search")}" value="${esc(s.query)}" ${busy() ? "disabled" : ""}>${s.query ? iconButton("clear-search", "clearSearch", "close") : ""}</div>
      ${s.mode ? `<div class="selection-toolbar"><button class="text-button selection-count" data-action="all" role="checkbox" aria-checked="${all ? "true" : s.selected.size ? "mixed" : "false"}" aria-label="${t(all ? "clearSelection" : "allResults")}" ${busy() || !rows.length ? "disabled" : ""}><span class="entry-check ${all ? "checked" : s.selected.size ? "mixed" : ""}" aria-hidden="true">${all ? icon("check") : s.selected.size ? icon("minus") : ""}</span>${t("selectedFraction", { count: s.selected.size, total: rows.length })}</button><button class="text-button" data-action="all" ${busy() || !rows.length ? "disabled" : ""}>${t(all ? "clearSelection" : "allResults")}</button></div>` : ""}
      <div class="entry-list" id="entry-list" tabindex="0" aria-label="${esc(title)}">${rows.length ? rows.map(item => {
        const checked = s.mode && s.selected.has(item.id), active = !s.mode && item.id === s.active;
        return `<button class="entry-row ${checked || active ? "selected" : ""}" data-entry="${item.id}" ${s.mode ? `role="checkbox" aria-checked="${checked}"` : `aria-pressed="${active}"`} aria-label="${esc(t(s.mode ? "selectEntry" : "openEntry", { path: path(item) }))}" ${busy() ? "disabled" : ""}>
          ${s.mode ? `<span class="entry-check ${checked ? "checked" : ""}" aria-hidden="true">${checked ? icon("check") : ""}</span>` : `<span class="avatar" aria-hidden="true">${esc(item.name.slice(0, 2).toUpperCase())}</span>`}<span class="entry-text"><span class="entry-name">${esc(item.name)}</span><span class="entry-path" title="${esc(path(item))}">${esc(path(item))}</span></span>${s.mode || s.scope === "trash" ? "" : `<span class="row-copy" data-copy="${item.id}" role="presentation">${icon("copy")}</span>`}</button>`;
      }).join("") : `<div class="empty-state">${icon("search")}<strong>${t(s.scope === "trash" && !s.query ? "trashEmpty" : "emptyResults")}</strong><p>${t(s.scope === "trash" && !s.query ? "trashHelp" : "emptyResultsDescription")}</p></div>`}</div>
      <footer class="library-footer"><span>${t(s.query ? "resultCount" : "entryCount", { count: rows.length, total: collection().length })}</span>${s.mode ? `<span class="muted">${t("selectedItems", { count: s.selected.size })}</span>` : ""}</footer></section>`;
  }

  function previewRow(item, suffix = "") {
    return `<div class="preview-row">${icon("key")}<span class="entry-text"><span class="entry-name">${esc(item.name)}</span><span class="entry-path">${esc(path(item))}</span></span>${suffix}</div>`;
  }

  function batchPanel() {
    const chosen = selected();
    const count = chosen.length;
    return `<div class="batch-panel"><div class="batch-heading">${count ? `<h2>${t("selectedCount", { count })}</h2><p class="batch-subtitle">${t("selectedFrom", { count: new Set(chosen.map(item => item.folder)).size })}</p>` : `<div class="empty-symbol">${icon("selection")}</div><h2>${t("chooseItems")}</h2><p class="batch-subtitle">${t("chooseItemsDescription")}</p>`}</div>
      <div class="batch-actions">${button("move", t("moveTo"), "primary", "folder", !count)}${button("trash", t("trashAction"), "secondary danger-text", "trash", !count)}</div>
      ${count ? `<section class="selected-preview"><div class="preview-heading"><h3>${t("selectedEntries")}</h3><span class="muted">${t("selectedItems", { count })}</span></div><div class="preview-list">${chosen.map(item => previewRow(item)).join("")}</div></section>` : ""}
      <div class="selection-help"><p>${t("selectHelp")}</p><p>${t("scopeHelp")}</p></div></div>`;
  }

  function detailPanel() {
    const item = s.scope === "trash" ? trashItems.find(entry => entry.id === s.active) : byID(s.active);
    if (!item) return `<div class="empty-state"><div class="empty-symbol">${icon(s.scope === "trash" ? "trash" : "key")}</div><h2>${t(s.scope === "trash" ? "trashEmpty" : "noDetail")}</h2><p>${t(s.scope === "trash" ? "trashHelp" : "noDetailDescription")}</p></div>`;
    if (s.scope === "trash") return `<div class="batch-panel"><div class="detail-heading"><div><h2>${esc(item.name)}</h2><p class="detail-path">${esc(path(item))}</p></div></div><p class="muted">${t("trashHelp")}</p>${button("restore", t("restore"), "primary", "restore")}</div>`;
    return `<div class="batch-panel"><header class="detail-heading"><div><h2>${esc(item.name)}</h2><p class="detail-path">${esc(path(item))}</p></div><div class="detail-actions">${iconButton("reveal", s.reveal ? "hidePassword" : "showPassword", "eye")}${iconButton("demo", "editEntry", "edit")}${iconButton("trash-one", "trashAction", "trash")}</div></header>
      <div class="field-row"><div><p class="field-label">${t("password")}</p><span class="field-value secret-value">${s.reveal ? "Example-only-2026" : "••••••••••••••"}</span></div>${iconButton("copy", "copyPassword", "copy")}</div>
      <div class="field-row"><div><p class="field-label">${t("username")}</p><span class="field-value">${esc(item.username)}</span></div>${iconButton("copy", "copyUsername", "copy")}</div>
      <div class="field-row"><div><p class="field-label">${t("website")}</p><span class="field-value">${esc(item.website)}</span></div></div><div class="detail-notes"><p class="field-label">${t("notes")}</p><p>${t("demoNotes")}</p></div>${button("demo", t("openWebsite"), "secondary", "external")}</div>`;
  }

  function resultPanel() {
    const r = s.result;
    if (r.kind === "partial") return `<div class="batch-panel"><div class="result-summary warning">${icon("warning")}<h2>${t("partialTitle", { success: r.success, failed: r.failed.length })}</h2><p class="batch-subtitle">${t("partialDescription")}</p></div><div class="result-list">${r.failed.map(failure => {
      const item = byID(failure.id);
      return `<div class="result-row"><div class="preview-heading"><strong>${esc(item.name)}</strong><span class="status-tag ${failure.review ? "warning" : ""}">${t(failure.review ? "needsReview" : "retryable")}</span></div><p class="entry-path">${esc(path(item))}</p><p class="result-description">${t(failure.reason)}</p>${failure.review ? `<button class="text-button" data-action="check-both">${t("checkBoth")}${icon("arrow")}</button>` : ""}</div>`;
    }).join("")}</div><div class="batch-actions">${button("retry", t("retryFailed"), "primary", "restore", !r.failed.some(f => !f.review))}${button("result-close", t("finish"))}</div><div class="selection-help"><p>${t("partialHelp")}</p></div></div>`;
    const moved = r.kind === "moved";
    return `<div class="batch-panel"><div class="result-summary success">${icon("checkCircle")}<h2>${t(moved ? "movedSuccess" : "trashedSuccess", { count: r.count })}</h2><p class="batch-subtitle">${t(moved ? "movedDestination" : "trashDescription", { folder: folderLabel(r.destination) })}</p></div><div class="batch-actions">${button(moved ? "destination" : "view-trash", t(moved ? "viewDestination" : "viewTrash"), "primary", moved ? "folder" : "trash")}${button("result-close", t("finish"))}</div><section class="selected-preview"><div class="preview-heading"><h3>${t("selectedEntries")}</h3><span class="muted">${t("selectedItems", { count: r.count })}</span></div>${r.originals.map(item => previewRow(moved ? { ...item, folder: r.destination } : item, `<span class="success-text">${icon("check")}</span>`)).join("")}</section>${moved ? "" : `<p class="selection-help">${t("trashLocalNotice")}</p>`}</div>`;
  }

  function progressPanel() {
    const p = s.progress;
    return `<div class="batch-panel"><div class="batch-heading"><h2>${t(p.action === "move" ? "progressMoving" : "progressTrashing")}</h2><p class="batch-subtitle">${t("progressHint")}</p></div><div class="progress-block"><div class="progress-meta"><span>${t("progressFraction", { done: p.done, total: p.total })}</span><span>${Math.round(p.done / p.total * 100)}%</span></div><div class="progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="${p.total}" aria-valuenow="${p.done}" aria-label="${t(p.action === "move" ? "progressMoving" : "progressTrashing")}"><div class="progress-fill" style="width:${p.done / p.total * 100}%"></div></div><p class="muted">${esc(path(byID(p.ids[Math.min(p.done, p.ids.length - 1)]) || seed[0]))}</p></div></div>`;
  }

  function modal() {
    if (!s.dialog) return "";
    if (s.dialog === "check") return `<dialog class="modal" aria-labelledby="modal-title"><header class="modal-header"><h2 id="modal-title">${t("checkBothTitle")}</h2></header><div class="modal-body"><p class="modal-description">${t("checkBothDescription")}</p><div class="path-preview"><p class="field-label">${t("originalPath")}</p><p>${esc(path(byID("li")))}</p><p class="field-label">${t("destinationPath")}</p><p>${esc(`${s.result.destination}/${byID("li").name}`)}</p></div></div><footer class="modal-footer">${button("cancel-dialog", t("back"), "primary")}</footer></dialog>`;
    const count = selected().length;
    if (s.dialog === "trash") return `<dialog class="modal trash-modal" aria-labelledby="modal-title" aria-describedby="modal-description"><header class="modal-header"><h2 id="modal-title">${t("trashTitle", { count })}</h2></header><div class="modal-body"><p class="modal-description" id="modal-description">${t("trashDescription")}</p><div class="confirm-preview">${selected().slice(0, 4).map(item => previewRow(item)).join("")}${count > 4 ? `<p class="muted">${t("moreItems", { count: count - 4 })}</p>` : ""}</div><p class="inline-notice">${icon("info")}${t("trashLocalNotice")}</p></div><footer class="modal-footer">${button("cancel-dialog", t("cancel"), "secondary", "", false, 'autofocus id="cancel-dialog"')}${button("confirm-trash", t("trashConfirm", { count }), "danger", "trash", !count)}</footer></dialog>`;
    const plan = movePlan();
    const folderOptions = ["", ...folders()];
    const candidate = s.folderQuery.trim().replace(/^\/+|\/+$/g, "");
    const canCreate = candidate && !folderOptions.includes(candidate) && !candidate.split("/").some(segment => [".", ".."].includes(segment));
    const shown = folderOptions.filter(folder => folderLabel(folder).toLocaleLowerCase().includes(s.folderQuery.trim().toLocaleLowerCase()));
    return `<dialog class="modal move-modal" aria-labelledby="modal-title" aria-describedby="modal-description"><header class="modal-header"><h2 id="modal-title">${t("moveShort")}</h2><p class="modal-description" id="modal-description">${t("moveDescription", { count })}</p></header><div class="modal-body"><p class="destination-label">${t("destination")}</p>${s.destination !== null ? `<button id="destination-trigger" class="destination-trigger" data-action="choose-folder" aria-expanded="${s.folderPickerOpen}">${icon("folder")}<span>${esc(folderLabel(s.destination))}</span>${icon("down")}</button>` : ""}${s.folderPickerOpen ? `<div class="folder-picker"><div class="folder-search search-wrap">${icon("search")}<input id="folder-search" placeholder="${t("folderSearch")}" aria-label="${t("folderSearchLabel")}" value="${esc(s.folderQuery)}"></div><div class="folder-list" role="group" aria-label="${t("chooseDestination")}">${shown.map(folder => `<button class="folder-option ${s.destination === folder ? "selected" : ""}" data-folder="${esc(folder)}" aria-pressed="${s.destination === folder}">${icon("folder")}<span class="folder-option-path">${esc(folderLabel(folder))}</span>${s.destination === folder ? icon("check") : ""}</button>`).join("")}${canCreate ? `<button class="folder-option folder-create" data-folder="${esc(candidate)}">${icon("folderPlus")}<span>${esc(t("createFolder", { folder: candidate }))}</span></button>` : ""}${!shown.length && !canCreate ? `<p class="muted">${t("noFolders")}</p>` : ""}</div></div>` : ""}
      ${s.destination === null ? `<p class="selection-help">${t("moveRule")}</p>` : `<div class="move-summary"><p class="muted">${t("moveRule")}</p><div class="move-counts"><span>${t("readyCount", { count: plan.ready.length })}</span>${plan.conflicts.length ? `<span class="danger-text">${t("conflictCount", { count: plan.conflicts.length })}</span>` : ""}${plan.unchanged.length ? `<span class="muted">${t("unchangedCount", { count: plan.unchanged.length })}</span>` : ""}</div></div>
      ${plan.conflicts.length ? `<section class="conflict-summary"><h3>${t("conflictHeading", { count: plan.conflicts.length })}</h3><p>${t("conflictDescription")}</p><div class="conflict-list">${plan.conflicts.map(item => `<div class="conflict-row">${icon("warning")}<div><strong>${esc(item.name)}</strong><span class="entry-path">${esc(path(item))}</span><span class="result-description">${t(item.reason)}</span></div></div>`).join("")}</div></section>` : ""}
      ${plan.ready.length ? `<section class="path-preview"><h3>${t("previewDestination")}</h3>${plan.ready.map(item => `<div class="path-preview-row"><span class="old-path">${esc(path(item))}</span>${icon("arrow")}<span class="new-path">${esc(s.destination ? `${s.destination}/${item.name}` : item.name)}</span></div>`).join("")}</section>` : `<p class="inline-notice">${icon("info")}${t("noMovable")}</p>`}
      ${s.destination.startsWith("工作/共享账号") ? `<p class="policy-notice">${icon("info")}${t("policyNotice")}</p>` : ""}`}</div>
      <footer class="modal-footer">${button("cancel-dialog", t("cancel"), "secondary", "", false, 'id="cancel-dialog"')}${plan.conflicts.length ? button("adjust-selection", t("resolveSelection"), "secondary") : ""}${button("confirm-move", t(plan.conflicts.length ? "skipMove" : "moveCount", { count: plan.ready.length }), "primary", "", !plan.ready.length)}</footer></dialog>`;
  }

  function render() {
    const previous = document.activeElement;
    const focusID = previous?.id;
    const range = previous instanceof HTMLInputElement ? [previous.selectionStart, previous.selectionEnd] : null;
    const listScroll = document.getElementById("entry-list")?.scrollTop || 0;
    const inspectorScroll = document.querySelector(".inspector-scroll")?.scrollTop || 0;
    const modalScroll = document.querySelector(".modal-body")?.scrollTop || 0;
    const wasDialog = Boolean(document.querySelector("dialog[open]"));
    document.documentElement.dataset.theme = theme;
    document.documentElement.lang = language;
    document.title = t("pageTitle");
    root.innerHTML = `${reviewBar()}<section class="app-window" aria-label="${t("brand")}"><header class="titlebar"><div class="titlebar-left">${iconButton("demo", "previous", "back", true)}${iconButton("demo", "next", "arrow", true)}<button class="icon-button sidebar-toggle" data-action="sidebar" aria-label="${t("toggleSidebar")}">${icon("sidebar")}</button><strong class="app-title">${t("passwordLibrary")}</strong></div><div class="titlebar-right"><span class="vault-name">${icon("folder")}${t("personalVault")}${icon("down")}</span><span class="sync-label">${t("savedLocally")}</span>${iconButton("demo", "lock", "lock")}</div></header><div class="workspace">${navigation()}${s.sidebar ? `<button class="sidebar-backdrop" data-action="sidebar" aria-label="${t("toggleSidebar")}"></button>` : ""}${list()}<section class="inspector" aria-live="polite"><div class="inspector-scroll">${s.progress ? progressPanel() : s.result ? resultPanel() : s.mode ? batchPanel() : detailPanel()}</div></section></div>${s.toast ? `<div class="toast-region" role="status"><div class="toast">${icon("checkCircle")}<span class="toast-copy">${t(s.toast.key, s.toast.values)}</span>${s.toast.action ? `<button class="text-button" data-action="${s.toast.action}">${t(s.toast.action === "destination" ? "viewDestination" : "viewTrash")}</button>` : ""}${iconButton("close-toast", "close", "close")}</div></div>` : ""}</section>${modal()}`;
    const modalNode = document.querySelector("dialog");
    if (modalNode) {
      modalNode.showModal();
      modalNode.addEventListener("cancel", event => { event.preventDefault(); closeDialog(); });
      modalNode.addEventListener("click", event => { if (event.target === modalNode) { const rect = modalNode.getBoundingClientRect(); if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) closeDialog(); } });
    }
    document.getElementById("entry-list").scrollTop = listScroll;
    document.querySelector(".inspector-scroll").scrollTop = inspectorScroll;
    const body = document.querySelector(".modal-body"); if (body) body.scrollTop = modalScroll;
    bind();
    if (focusID && document.getElementById(focusID) && (!modalNode || wasDialog || focusID === "folder-search")) {
      const next = document.getElementById(focusID); next.focus({ preventScroll: true });
      if (range && next instanceof HTMLInputElement) next.setSelectionRange(...range);
    } else if (modalNode && !wasDialog) (s.dialog === "trash" ? document.getElementById("cancel-dialog") : document.getElementById("folder-search") || document.getElementById("destination-trigger"))?.focus({ preventScroll: true });
  }

  function toast(key, values = {}, action = null) {
    clearTimeout(toastTimer); s.toast = { key, values, action }; render();
    toastTimer = setTimeout(() => { s.toast = null; render(); }, 5000);
  }

  function closeDialog() { s.dialog = null; render(); document.querySelector(`[data-action="${s.returnAction || "move"}"]`)?.focus({ preventScroll: true }); }
  function clearForScope() { s.selected.clear(); s.result = null; anchor = null; }
  function enterDialog(kind) { s.dialog = kind; s.returnAction = kind; s.result = null; if (kind === "move") { s.destination = null; s.folderQuery = ""; s.folderPickerOpen = true; } render(); }

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
    const originals = ids.map(id => ({ ...byID(id) }));
    const conflicts = plan?.conflicts.map(item => item.id) || [];
    s.dialog = null; s.progress = { action, ids, total: ids.length, done: 0 }; s.toast = null; render();
    const step = () => {
      if (!s.progress) return;
      s.progress.done++; render();
      if (s.progress.done < ids.length) timer = setTimeout(step, 320);
      else timer = setTimeout(() => {
        applySuccess(action, ids, destination);
        if (conflicts.length) {
          s.mode = true; s.selected = new Set(conflicts); s.result = null;
          s.toast = { key: "movedSuccess", values: { count: originals.length }, action: "destination" };
          s.lastDestination = destination;
        }
        render();
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
      case "mode": s.mode = !s.mode; s.selected.clear(); s.result = null; render(); break;
      case "all": {
        const ids = visible().map(item => item.id), all = ids.every(id => s.selected.has(id));
        s.mode = true; s.result = null; ids.forEach(id => all ? s.selected.delete(id) : s.selected.add(id)); render(); break;
      }
      case "clear-search": s.query = ""; clearForScope(); render(); document.getElementById("search")?.focus(); break;
      case "move": enterDialog("move"); break;
      case "choose-folder": s.folderPickerOpen = !s.folderPickerOpen; render(); if (s.folderPickerOpen) document.getElementById("folder-search")?.focus({ preventScroll: true }); break;
      case "trash": enterDialog("trash"); break;
      case "trash-one": s.mode = true; s.selected = new Set([s.active]); enterDialog("trash"); break;
      case "cancel-dialog": closeDialog(); break;
      case "adjust-selection": s.dialog = null; render(); break;
      case "confirm-move": startOperation("move"); break;
      case "confirm-trash": startOperation("trash"); break;
      case "reveal": s.reveal = !s.reveal; render(); break;
      case "destination": {
        const destination = s.result?.destination ?? s.lastDestination ?? "";
        s.scope = destination || "__root__"; s.query = ""; s.mode = false; s.result = null; s.toast = null; s.selected.clear(); s.active = items.find(item => item.folder === destination)?.id; render(); break;
      }
      case "view-trash": s.scope = "trash"; s.mode = false; s.result = null; s.toast = null; s.query = ""; s.active = trashItems[0]?.id; render(); break;
      case "result-close": s.result = null; s.mode = false; s.selected.clear(); s.active = null; render(); break;
      case "check-both": s.dialog = "check"; s.returnAction = "check-both"; render(); break;
      case "retry": {
        const retry = s.result.failed.filter(failure => !failure.review);
        const r = s.result;
        retry.forEach(failure => { byID(failure.id).folder = r.destination; s.selected.delete(failure.id); });
        r.success += retry.length; r.failed = r.failed.filter(failure => failure.review);
        toast("movedSuccess", { count: retry.length }); break;
      }
      case "restore": {
        const item = trashItems.find(entry => entry.id === s.active);
        if (item && !items.some(existing => path(existing) === path(item))) { items.push(item); trashItems = trashItems.filter(entry => entry.id !== item.id); s.active = trashItems[0]?.id; toast("restored", { count: 1 }); }
        break;
      }
      case "close-toast": s.toast = null; render(); break;
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
      s.scope = node.dataset.scope; s.query = ""; clearForScope(); s.active = null; s.sidebar = false;
      if (s.scope === "trash") { s.mode = false; s.active = trashItems[0]?.id; }
      render(); document.getElementById("entry-list").scrollTop = 0;
    }));
    root.querySelectorAll("[data-entry]").forEach(node => node.addEventListener("click", event => {
      if (event.target.closest("[data-copy]")) { toast("copiedDemo"); return; }
      toggleEntry(node.dataset.entry, event);
    }));
    document.getElementById("search")?.addEventListener("input", event => { s.query = event.target.value; clearForScope(); render(); });
    document.getElementById("folder-search")?.addEventListener("input", event => { s.folderQuery = event.target.value; render(); });
    root.querySelectorAll("[data-folder]").forEach(node => node.addEventListener("click", () => { s.destination = node.dataset.folder; s.folderPickerOpen = false; s.folderQuery = ""; render(); document.getElementById("destination-trigger")?.focus({ preventScroll: true }); }));
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
