(() => {
  const params = new URLSearchParams(location.search);
  const presentation = document.body.dataset.presentation === "design";
  const catalogs = presentation ? globalThis.PASSPINE_CLIENT_DESIGN_COPY : globalThis.PASSPINE_THREE_COLUMN_COPY;
  let language = params.get("lang") === "en" ? "en" : "zh-CN";
  let c = catalogs[language];
  const states = ["detail","empty","folder","collapsed","edit","drafts","trash"];
  const initial = states.includes(params.get("state")) ? params.get("state") : document.body.dataset.state || "detail";
  const items = [
    {id:1,kind:"entry",name:"GitHub 工作账号",folder:"开发",username:"pine@example.test",password:"Example-only-2026",website:"https://github.com",notes:"工作与开源项目使用的账号。\n\n恢复代码另行保存在离线备份中。"},
    {id:2,kind:"entry",name:"Cloudflare",folder:"开发",username:"infra@example.test",password:"Cloudflare-demo-only",website:"https://dash.cloudflare.com",notes:"域名与个人项目。"},
    {id:3,kind:"entry",name:"Apple",folder:"生活",username:"hello@example.test",password:"Apple-demo-only",website:"https://account.apple.com",notes:""},
    {id:4,kind:"entry",name:"Figma 设计团队",folder:"工作",username:"design@example.test",password:"Figma-demo-only",website:"https://figma.com",notes:"设计团队共享工作空间。"},
    {id:5,kind:"entry",name:"Linear",folder:"工作",username:"team@example.test",password:"Linear-demo-only",website:"https://linear.app",notes:""},
    {id:6,kind:"entry",name:"NAS 家庭存储",folder:"家庭/设备",username:"admin",password:"NAS-demo-only",website:"https://nas.example.test",notes:""},
    {id:7,kind:"entry",name:"Notion 团队工作空间与长期项目资料",folder:"工作/共享账号",username:"docs@example.test",password:"Notion-demo-only",website:"https://notion.so",notes:"团队工作空间使用的账号。\n\n项目资料按季度整理，历史资料放在归档目录。\n\n共享信息由团队负责人维护。\n\n工作区内的页面与资料由使用者管理。\n\n历史项目的访问权限在每个季度末复核。\n\n离职成员的账号由负责人统一移除。\n\n重要文档每月导出备份。\n\n共享账号的恢复信息保存在离线介质中。\n\n完成后可继续选择其他条目。"},
    {id:8,kind:"entry",name:"Vercel",folder:"开发",username:"web@example.test",password:"Vercel-demo-only",website:"https://vercel.com",notes:""},
    {id:9,kind:"entry",name:"Home Assistant",folder:"家庭/设备",username:"home",password:"Home-demo-only",website:"https://home.example.test",notes:""},
    {id:10,kind:"entry",name:"邮件",folder:"生活",username:"pine@example.test",password:"Mail-demo-only",website:"https://mail.example.test",notes:""},
    {id:11,kind:"draft",name:"AWS 项目账号",folder:"开发",username:"cloud@example.test",password:"AWS-demo-only",website:"https://console.aws.amazon.com",notes:"项目账号的未完成编辑。"},
    {id:12,kind:"trash",name:"旧项目邮箱",folder:"工作/归档",username:"archive@example.test",password:"Archive-demo-only",website:"https://mail.example.test",notes:"旧项目保留的邮箱账号。"}
  ];
  const fields = ["name","folder","password","username","website","notes"];
  const escape = value => String(value).replace(/[&<>"']/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[char]));
  const text = (key, values = {}) => c[key].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ""));
  const path = item => item.folder ? `${item.folder}/${item.name}` : item.name;
  const url = file => {
    const query = new URLSearchParams({ lang: language });
    if (!presentation) return `${file}?${query}`;
    if (file === "../index.html") return `main-window-wireframe/index.html?${query}`;
    query.set("state", file === "index.html" ? "detail" : file.replace(/\.html$/, ""));
    if (params.get("theme")) query.set("theme", params.get("theme"));
    return `client.html?${query}`;
  };
  const narrow = matchMedia("(max-width: 960px)");
  let scope = initial === "folder" ? "folder:工作" : ["drafts","trash"].includes(initial) ? initial : "all";
  let selected = initial === "empty" ? null : initial === "folder" ? 4 : initial === "drafts" ? 11 : initial === "trash" ? 12 : 1;
  let editing = initial === "edit";
  let draft = editing ? {...items[0]} : null;
  let isNew = false;
  let pending = null;
  let revealed = false;
  let statusKey = "";
  let errorKey = "";
  let searchValue = "";
  let collapsed = params.get("sidebar") === "collapsed" || initial === "collapsed";
  let overlay = narrow.matches && params.get("sidebar") === "expanded";
  let listScroll = 0;
  let panelScroll = 0;
  const splitLimits = { minimum: 240, maximum: 480, detailMinimum: 360 };
  const libraryWidthStorageKey = "passpine.design.library-width";
  let preferredLibraryWidth = readLibraryWidth();
  let splitObserver = null;
  const entry = () => items.find(item => item.id === selected);
  const dirty = () => editing && (isNew || fields.some(key => draft[key] !== entry()[key]));
  const currentFolder = () => scope.startsWith("folder:") ? scope.slice(7) : "";
  const scopeTitle = value => value === "all" ? c.allEntries : value === "drafts" ? c.drafts : value === "trash" ? c.trash : value.slice(7);
  const inFolder = (item, folder) => item.folder === folder || item.folder.startsWith(`${folder}/`);
  const collection = () => items.filter(item => scope === "drafts" ? item.kind === "draft" : scope === "trash" ? item.kind === "trash" : item.kind === "entry" && (!currentFolder() || inFolder(item,currentFolder())));
  const icon = name => `<svg class="icon" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${{
    sidebar:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16M6 8h.01M6 11h.01"/>',
    all:'<rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/>',
    folder:'<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1Z"/>',
    draft:'<path d="M14 3H5v18h14V8Z"/><path d="M14 3v5h5M8 12h8M8 16h5"/>',
    trash:'<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/>',
    search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',
    plus:'<path d="M12 5v14M5 12h14"/>',
    key:'<circle cx="8" cy="9" r="5"/><path d="m12 13 8 8M16 17l3-3M18 19l3-3"/>',
    close:'<path d="m6 6 12 12M6 18 18 6"/>'
  }[name]}</svg>`;

  function shell() {
    document.documentElement.lang = language;
    document.title = c.pageTitle;
    document.getElementById("root").innerHTML = `
      <main class="review">
        <div class="review-heading"><span class="eyebrow">${c.eyebrow}</span><h1>${c.heading}</h1></div>
        <p class="disclaimer">${c.disclaimer}</p>
        <nav class="review-nav" aria-label="${escape(c.heading)}">${c.stages.map(stage => `<a href="${url(stage.file)}" ${stage.file === (initial === "detail" ? "index.html" : `${initial}.html`) ? 'aria-current="page"' : ""}>${stage.label}</a>`).join("")}<a class="guard-link" href="${url("../index.html")}">${c.comparePrevious}</a></nav>
        <div class="window-scroll"><section class="window" aria-label="Passpine">
          <header class="titlebar"><div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div><button class="quiet icon-button sidebar-toggle" id="sidebar-toggle" aria-controls="navigation">${icon("sidebar")}</button><span class="app-name">Passpine</span><span class="vault">${c.vault}</span><span class="sync">${c.synced}</span></header>
          <div class="workspace">
            <button class="sidebar-scrim" id="sidebar-scrim" aria-label="${escape(c.closeSidebar)}" hidden></button>
            <aside class="navigation" id="navigation" aria-label="${escape(c.navLabel)}"></aside>
            <section class="library" id="library" aria-labelledby="scope-title">
              <header class="library-heading"><h2 id="scope-title"></h2><button class="quiet icon-button" id="new-entry" aria-label="${escape(c.newEntry)}" title="${escape(c.newEntry)}">${icon("plus")}</button></header>
              <div class="list-tools"><div class="search">${icon("search")}<input id="search" aria-label="${escape(c.collectionSearch)}" placeholder="${escape(c.search)}" value="${escape(searchValue)}"></div></div>
              <div class="entry-list" id="entry-list"></div>
              <footer class="library-footer"><span class="count" id="count"></span></footer>
            </section>
            <div class="column-splitter" id="column-splitter" role="separator" tabindex="0" aria-orientation="vertical" aria-controls="library inspector" aria-label="${escape(c.resizeColumns)}" aria-describedby="column-splitter-help" title="${escape(c.resizeColumnsHint)}"><span aria-hidden="true"></span></div>
            <span class="visually-hidden" id="column-splitter-help">${c.resizeColumnsHelp}</span>
            <aside class="inspector" id="inspector" aria-label="${escape(c.detailLabel)}"></aside>
          </div>
          <div class="modal-layer" id="modal" hidden></div>
        </section></div>
        <div class="annotation" id="annotation"></div>
        <div class="review-footer"><p class="prototype-note">${c.interactionHint}</p><div class="language" aria-label="${escape(c.languageLabel)}"><a href="?lang=zh-CN" data-language="zh-CN" ${language === "zh-CN" ? 'aria-current="page"' : ""}>${c.languageChinese}</a><a href="?lang=en" data-language="en" ${language === "en" ? 'aria-current="page"' : ""}>${c.languageEnglish}</a></div></div>
        <p class="prototype-note">${c.prototypeOnly}</p>
      </main>`;
    document.getElementById("search").addEventListener("input", event => { searchValue = event.target.value; drawList(); });
    document.getElementById("new-entry").addEventListener("click", () => request({type:"new"}));
    document.getElementById("sidebar-toggle").addEventListener("click", () => {
      if (narrow.matches) overlay = !overlay;
      else collapsed = !collapsed;
      sidebar();
      notes();
    });
    document.getElementById("sidebar-scrim").addEventListener("click", closeOverlay);
    document.querySelectorAll("[data-language]").forEach(link => link.addEventListener("click", event => {
      event.preventDefault();
      if (link.dataset.language === language) return;
      listScroll = document.getElementById("entry-list").scrollTop;
      panelScroll = document.querySelector(".panel-body")?.scrollTop || 0;
      language = link.dataset.language;
      c = catalogs[language];
      const query = new URLSearchParams(location.search);
      query.set("lang",language);
      history.replaceState(null,"",`?${query}`);
      shell();
    }));
    document.getElementById("entry-list").addEventListener("keydown", event => {
      if (!["ArrowDown","ArrowUp"].includes(event.key)) return;
      const buttons = Array.from(document.querySelectorAll("[data-entry]"));
      const index = buttons.indexOf(document.activeElement);
      const next = buttons[index + (event.key === "ArrowDown" ? 1 : -1)];
      if (next) { event.preventDefault(); next.focus(); next.click(); if (!pending) document.querySelector(`[data-entry="${selected}"]`)?.focus(); }
    });
    navigation(); drawList(); drawPanel(); sidebar();
    bindColumnSplitter();
    document.getElementById("entry-list").scrollTop = listScroll;
    if (pending) drawModal();
  }

  function sidebar() {
    const shown = narrow.matches ? overlay : !collapsed;
    const win = document.querySelector(".window");
    win.classList.toggle("sidebar-collapsed", narrow.matches || collapsed);
    win.classList.toggle("sidebar-overlay", narrow.matches && overlay);
    const nav = document.getElementById("navigation");
    nav.inert = !shown;
    nav.setAttribute("aria-hidden", String(!shown));
    const toggle = document.getElementById("sidebar-toggle");
    toggle.setAttribute("aria-expanded", String(shown));
    toggle.setAttribute("aria-label", shown ? c.collapseSidebar : c.expandSidebar);
    toggle.title = shown ? c.collapseSidebar : c.expandSidebar;
    document.getElementById("sidebar-scrim").hidden = !(narrow.matches && overlay);
    resizeColumns();
  }
  function closeOverlay() { overlay = false; sidebar(); document.getElementById("sidebar-toggle").focus(); }

  function columnBounds() {
    const workspace = document.querySelector(".workspace");
    const navigationWidth = !narrow.matches && !collapsed ? document.getElementById("navigation").getBoundingClientRect().width : 0;
    const available = workspace.getBoundingClientRect().width - navigationWidth;
    return { minimum: splitLimits.minimum, maximum: Math.max(splitLimits.minimum, Math.min(splitLimits.maximum, Math.floor(available - splitLimits.detailMinimum))) };
  }
  function readLibraryWidth() {
    try {
      const width = Number(localStorage.getItem(libraryWidthStorageKey));
      return Number.isInteger(width) && width >= splitLimits.minimum && width <= splitLimits.maximum ? width : null;
    } catch { return null; }
  }
  function saveLibraryWidth() {
    try { localStorage.setItem(libraryWidthStorageKey, String(preferredLibraryWidth ?? 0)); }
    catch {}
  }
  function resizeColumns() {
    const splitter = document.getElementById("column-splitter");
    if (!splitter) return;
    const bounds = columnBounds();
    const desired = preferredLibraryWidth ?? (narrow.matches ? 280 : 300);
    const width = Math.max(bounds.minimum, Math.min(bounds.maximum, desired));
    document.querySelector(".workspace").style.setProperty("--library-width", `${width}px`);
    splitter.setAttribute("aria-valuemin", String(bounds.minimum));
    splitter.setAttribute("aria-valuemax", String(bounds.maximum));
    splitter.setAttribute("aria-valuenow", String(width));
    splitter.setAttribute("aria-valuetext", text("listWidthValue", { width }));
  }
  function setLibraryWidth(width) {
    const bounds = columnBounds();
    preferredLibraryWidth = Math.round(Math.max(bounds.minimum, Math.min(bounds.maximum, width)));
    saveLibraryWidth();
    resizeColumns();
  }
  function bindColumnSplitter() {
    splitObserver?.disconnect();
    const splitter = document.getElementById("column-splitter");
    const workspace = document.querySelector(".workspace");
    let drag = null;
    const stopDrag = event => {
      if (!drag || event.pointerId !== drag.pointerId) return;
      drag = null;
      workspace.classList.remove("resizing-columns");
      if (splitter.hasPointerCapture(event.pointerId)) splitter.releasePointerCapture(event.pointerId);
    };
    splitter.addEventListener("pointerdown", event => {
      if (event.button !== 0) return;
      event.preventDefault();
      splitter.focus();
      drag = { pointerId: event.pointerId, startX: event.clientX, width: Number(splitter.getAttribute("aria-valuenow")) };
      splitter.setPointerCapture(event.pointerId);
      workspace.classList.add("resizing-columns");
    });
    splitter.addEventListener("pointermove", event => {
      if (drag && event.pointerId === drag.pointerId) setLibraryWidth(drag.width + event.clientX - drag.startX);
    });
    for (const type of ["pointerup", "pointercancel", "lostpointercapture"]) splitter.addEventListener(type, stopDrag);
    splitter.addEventListener("dblclick", () => { preferredLibraryWidth = null; saveLibraryWidth(); resizeColumns(); });
    splitter.addEventListener("keydown", event => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const bounds = columnBounds(), current = Number(splitter.getAttribute("aria-valuenow"));
      const step = event.shiftKey ? 40 : 10;
      setLibraryWidth(event.key === "Home" ? bounds.minimum : event.key === "End" ? bounds.maximum : current + (event.key === "ArrowRight" ? step : -step));
    });
    splitObserver = new ResizeObserver(resizeColumns);
    splitObserver.observe(workspace);
    resizeColumns();
  }

  function navigation() {
    const folders = new Set();
    const active = items.filter(item => item.kind === "entry");
    active.forEach(item => {
      const root = item.folder.split("/")[0];
      if (root) folders.add(root);
    });
    const navItem = (value,label,count,type,depth=0) => `<button class="nav-item depth-${depth} ${scope === value ? "selected" : ""}" data-scope="${escape(value)}" aria-current="${scope === value ? "true" : "false"}" aria-label="${escape(text("navCount",{name:label,count}))}" title="${escape(label)}">${icon(type)}<span class="nav-label">${escape(label)}</span><span class="nav-count">${count}</span></button>`;
    document.getElementById("navigation").innerHTML = `<div class="navigation-top"><span class="nav-caption">${c.navLabel}</span></div><div class="navigation-main">${navItem("all",c.allEntries,active.length,"all")}<div class="nav-group-title">${c.foldersLabel}</div><div class="folder-tree">${Array.from(folders).sort((a,b)=>a.localeCompare(b,"zh-CN")).map(folder=>navItem(`folder:${folder}`,folder,active.filter(item=>inFolder(item,folder)).length,"folder")).join("")}</div></div><div class="navigation-bottom">${navItem("drafts",c.drafts,items.filter(item=>item.kind === "draft").length,"draft")}${navItem("trash",c.trash,items.filter(item=>item.kind === "trash").length,"trash")}</div>`;
    document.querySelectorAll("[data-scope]").forEach(button => button.addEventListener("click", () => {
      if (button.dataset.scope !== scope) request({type:"scope",target:button.dataset.scope});
      else if (overlay) closeOverlay();
    }));
  }

  function drawList() {
    const list = document.getElementById("entry-list");
    const scroll = list.scrollTop;
    const current = collection();
    const query = searchValue.trim().toLowerCase();
    const visible = current.filter(item=>path(item).toLowerCase().includes(query));
    const matchCount = visible.length;
    const retained = editing && !isNew && !visible.some(item=>item.id === selected);
    if (retained) visible.unshift(entry());
    document.getElementById("scope-title").textContent = scopeTitle(scope);
    document.getElementById("scope-title").title = currentFolder() ? text("folderScope",{folder:currentFolder()}) : scopeTitle(scope);
    document.getElementById("new-entry").hidden = ["drafts","trash"].includes(scope);
    document.getElementById("count").textContent = retained ? text("retainedCount",{count:matchCount}) : query ? text("countFiltered",{visible:matchCount,total:current.length}) : text("footerCount",{count:current.length});
    list.innerHTML = visible.length ? visible.map(item => `<button class="entry ${item.id === selected ? "selected" : ""}" data-entry="${item.id}" aria-pressed="${item.id === selected}" aria-label="${escape(text("entryLabel",item))}" title="${escape(path(item))}"><span class="monogram" aria-hidden="true">${escape(item.name.slice(0,2).toUpperCase())}</span><span class="entry-copy"><span class="entry-name">${escape(item.name)}</span><span class="entry-subtitle">${escape(item.folder || item.username)}</span></span><span class="arrow" aria-hidden="true">›</span></button>`).join("") : `<div class="empty"><p>${c.noMatchesTitle}</p><p>${c.noMatchesBody}</p>${query ? `<button id="clear-search">${c.showAll}</button>` : ""}</div>`;
    list.scrollTop = scroll;
    list.querySelectorAll("[data-entry]").forEach(button => button.addEventListener("click", () => {
      const target = Number(button.dataset.entry);
      if (target !== selected || isNew) request({type:"select",target});
    }));
    document.getElementById("clear-search")?.addEventListener("click", () => { searchValue=""; document.getElementById("search").value=""; drawList(); });
    notes();
  }

  function drawPanel() {
    const inspector = document.getElementById("inspector");
    const item = isNew ? draft : entry();
    inspector.classList.toggle("editor",editing);
    inspector.setAttribute("aria-label",editing ? c.editorLabel : c.detailLabel);
    if (!item) {
      const prefix = scope === "drafts" ? "emptyDraft" : scope === "trash" ? "emptyTrash" : "emptySelection";
      inspector.innerHTML = `<div class="inspector-empty"><div class="empty-symbol">${icon(scope === "drafts" ? "draft" : scope === "trash" ? "trash" : "key")}</div><h2>${c[`${prefix}Title`]}</h2><p>${c[`${prefix}Body`]}</p></div>`;
      notes(); return;
    }
    const recovery = item.kind !== "entry" && !editing;
    inspector.innerHTML = `<header class="inspector-heading"><div class="heading-text"><h2>${escape(isNew ? c.newEntry : item.name)}</h2><p>${escape(path(item))}</p></div><div class="heading-actions">${editing ? `<span class="editor-state" id="edit-state">${dirty() ? c.changed : c.unchanged}</span>` : recovery ? "" : `<button id="edit">${c.edit}</button>`}${!editing ? `<button class="quiet close" id="clear-selection" aria-label="${escape(c.clearSelection)}" title="${escape(c.clearSelection)}">${icon("close")}</button>` : ""}</div></header>${editing ? editorHTML() : detailsHTML(item,recovery)}`;
    document.getElementById("clear-selection")?.addEventListener("click",()=>request({type:"clear"}));
    if (editing) {
      inspector.querySelectorAll("[data-field]").forEach(input => input.addEventListener("input", () => {
        draft[input.dataset.field] = input.value;
        errorKey = "";
        document.getElementById("edit-state").textContent = dirty() ? c.changed : c.unchanged;
        document.getElementById("save").disabled = !dirty();
        document.getElementById("form-error").textContent = "";
        inspector.querySelectorAll("[aria-invalid]").forEach(field=>field.removeAttribute("aria-invalid"));
      }));
      document.getElementById("cancel").addEventListener("click",()=>request({type:"cancel"}));
      document.getElementById("save").addEventListener("click",()=>save());
    } else {
      document.getElementById("edit")?.addEventListener("click",()=>{editing=true;draft={...item};statusKey="";panelScroll=0;drawPanel();document.querySelector('[data-field="name"]').focus();});
      document.getElementById("reveal")?.addEventListener("click",()=>{revealed=!revealed;document.getElementById("password-value").textContent=revealed ? item.password : "••••••••••••";document.getElementById("reveal").textContent=revealed ? c.hide : c.show;});
      inspector.querySelectorAll("[data-copy]").forEach(button=>button.addEventListener("click",()=>{statusKey="mockCopied";document.getElementById("panel-status").textContent=c[statusKey];}));
      document.getElementById("restore")?.addEventListener("click",()=>{
        const wasDraft = item.kind === "draft";
        item.kind="entry";scope="all";searchValue="";document.getElementById("search").value="";
        statusKey="mockRestored";editing=wasDraft;draft=wasDraft?{...item}:null;
        navigation();drawList();drawPanel();
      });
    }
    if (errorKey) {
      const invalid = errorKey === "folderInvalid" ? "folder" : "name";
      inspector.querySelector(`[data-field="${invalid}"]`)?.setAttribute("aria-invalid","true");
    }
    const body = inspector.querySelector(".panel-body");
    if(body) body.scrollTop=panelScroll;
    notes();
  }

  function detailsHTML(item,recovery) {
    const field = (key,value,secret=false) => `<div class="credential"><div class="field-title">${c[key]}</div><div class="credential-content"><span class="value ${secret ? "secret" : ""}" ${secret ? 'id="password-value"' : ""}>${escape(value)}</span>${secret ? `<button id="reveal">${revealed ? c.hide : c.show}</button>` : ""}<button data-copy="${key}" aria-label="${escape(text("copyDemo",{field:c[key]}))}">${c.copy}</button></div></div>`;
    return `<div class="panel-body">${recovery ? `<p class="collection-hint">${item.kind === "draft" ? c.draftDescription : c.trashDescription}</p>` : ""}${field("password",revealed ? item.password : "••••••••••••",true)}${item.username ? field("username",item.username) : ""}${item.website ? field("website",item.website) : ""}${item.notes ? `<div class="notes"><div class="field-title">${c.notes}</div><p>${escape(item.notes)}</p></div>` : ""}</div><footer class="panel-footer ${recovery ? "" : "view-footer"}"><span class="status" id="panel-status" role="status">${statusKey ? c[statusKey] : ""}</span>${recovery ? `<button class="primary" id="restore">${item.kind === "draft" ? c.restoreDraft : c.restoreTrash}</button>` : ""}</footer>`;
  }
  function editorHTML() {
    const row = key => `<label class="form-row"><span>${c[key]}</span>${key === "notes" ? `<textarea data-field="${key}">${escape(draft[key])}</textarea>` : `<input data-field="${key}" type="${key === "password" ? "password" : "text"}" value="${escape(draft[key])}" autocomplete="off" aria-describedby="form-error">`}</label>`;
    return `<div class="panel-body editor-form">${fields.map(row).join("")}<div class="form-error" id="form-error" role="alert">${errorKey ? c[errorKey] : ""}</div></div><footer class="panel-footer"><span class="status" role="status">${statusKey ? c[statusKey] : ""}</span><button id="cancel">${c.cancel}</button><button class="primary" id="save" ${dirty() ? "" : "disabled"}>${isNew ? c.create : c.save}</button></footer>`;
  }

  function notes() {
    const hidden = narrow.matches ? !overlay : collapsed;
    const key = editing ? isNew ? "createNote" : "editNote" : hidden ? "collapseNote" : scope === "drafts" ? "draftNote" : scope === "trash" ? "trashNote" : currentFolder() ? "folderNote" : selected ? "detailNote" : "listNote";
    document.getElementById("annotation").textContent = pending ? c.guardNote : c[key];
  }
  function request(action) {
    if (dirty()) {pending=action;drawModal();return;}
    execute(action);
  }
  function execute(action) {
    const previousSelected=selected;
    editing=false;isNew=false;draft=null;pending=null;revealed=false;statusKey="";errorKey="";panelScroll=0;
    closeModal(false);
    if(action.type === "scope") {scope=action.target;selected=null;searchValue="";document.getElementById("search").value="";listScroll=0;document.getElementById("entry-list").scrollTop=0;}
    else if(action.type === "select") selected=action.target;
    else if(action.type === "clear") selected=null;
    else if(action.type === "cancel") selected=previousSelected;
    else if(action.type === "new") {selected=null;isNew=true;editing=true;draft={id:Math.max(...items.map(item=>item.id))+1,kind:"entry",name:c.newEntryName,folder:currentFolder(),password:"",username:"",website:"",notes:""};}
    if(overlay) overlay=false;
    navigation();drawList();drawPanel();sidebar();
  }
  function save(action=null) {
    draft.name=draft.name.trim();draft.folder=draft.folder.trim();
    errorKey = !draft.name ? "nameRequired" : draft.name.includes("/") ? "nameInvalid" : /(^\/|\/$|\/\/)/.test(draft.folder) ? "folderInvalid" : items.some(item=>item.kind === "entry" && item.id !== selected && path(item) === path(draft)) ? "duplicate" : "";
    if(errorKey) {pending=null;closeModal(false);drawPanel();document.querySelector('[aria-invalid="true"]')?.focus();return;}
    if(isNew) {items.push({...draft});selected=draft.id;}
    else Object.assign(entry(),draft);
    if(currentFolder() && !inFolder(entry(),currentFolder())) {scope="all";searchValue="";document.getElementById("search").value="";}
    editing=false;isNew=false;draft=null;pending=null;statusKey="saved";errorKey="";panelScroll=0;
    closeModal(false);navigation();drawList();drawPanel();
    if(action) execute(action);
  }
  function drawModal() {
    const name = isNew ? draft.name : entry().name;
    const message = ["scope","new"].includes(pending.type) ? text("scopeGuard",{name,target:pending.type === "new" ? c.newEntry : scopeTitle(pending.target)}) : pending.type === "select" ? text("guardSwitch",{name,target:items.find(item=>item.id===pending.target).name}) : pending.type === "cancel" ? text("cancelGuard",{name}) : text("guardClose",{name});
    const modal=document.getElementById("modal");
    modal.hidden=false;
    modal.innerHTML=`<section class="dialog" role="dialog" aria-modal="true" aria-labelledby="guard-title" aria-describedby="guard-message"><h2 id="guard-title">${c.guardTitle}</h2><p id="guard-message">${escape(message)}</p><div class="dialog-actions"><button class="primary" id="save-continue">${c.saveContinue}</button><button id="discard">${c.discard}</button><button id="keep-editing">${c.keepEditing}</button></div></section>`;
    document.querySelector(".workspace").inert=true;
    document.querySelector(".titlebar").inert=true;
    document.getElementById("save-continue").addEventListener("click",()=>save(pending));
    document.getElementById("discard").addEventListener("click",()=>execute(pending));
    document.getElementById("keep-editing").addEventListener("click",()=>closeModal());
    document.getElementById("keep-editing").focus();
    notes();
  }
  function closeModal(focus=true) {
    pending=null;
    const modal=document.getElementById("modal");
    modal.hidden=true;modal.innerHTML="";
    document.querySelector(".workspace").inert=false;
    document.querySelector(".titlebar").inert=false;
    if(focus) {notes();document.getElementById("save")?.focus();}
  }
  document.addEventListener("keydown",event=>{
    if(event.key === "Escape") {if(pending){event.preventDefault();closeModal();}else if(overlay){event.preventDefault();closeOverlay();}}
    if(event.key === "Tab" && pending) {
      const buttons=Array.from(document.querySelectorAll("#modal button"));
      const first=buttons[0],last=buttons.at(-1);
      if(event.shiftKey && document.activeElement===first){event.preventDefault();last.focus();}
      else if(!event.shiftKey && document.activeElement===last){event.preventDefault();first.focus();}
    }
  });
  narrow.addEventListener("change",()=>{overlay=false;sidebar();notes();});
  shell();
})();
