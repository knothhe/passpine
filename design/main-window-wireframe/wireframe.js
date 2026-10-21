(() => {
  const params = new URLSearchParams(location.search);
  let language = params.get("lang") === "en" ? "en" : "zh-CN";
  let c = globalThis.PASSPINE_WIREFRAME_COPY[language];
  const text = (key, values = {}) => c[key].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? ""));
  const escape = value => String(value).replace(/[&<>"']/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[char]));
  const initial = document.body.dataset.state || "detail";
  const entries = [
    {id:1,name:"GitHub 工作账号",folder:"开发",username:"pine@example.test",password:"Example-only-2026",website:"https://github.com",notes:"工作与开源项目使用的账号。"},
    {id:2,name:"Cloudflare",folder:"开发",username:"infra@example.test",password:"Cloudflare-demo-only",website:"https://dash.cloudflare.com",notes:"域名与个人项目。"},
    {id:3,name:"Apple",folder:"生活",username:"hello@example.test",password:"Apple-demo-only",website:"https://account.apple.com",notes:""},
    {id:4,name:"Figma 设计团队",folder:"工作",username:"design@example.test",password:"Figma-demo-only",website:"https://figma.com",notes:""},
    {id:5,name:"Linear",folder:"工作",username:"team@example.test",password:"Linear-demo-only",website:"https://linear.app",notes:""},
    {id:6,name:"NAS 家庭存储",folder:"家庭/设备",username:"admin",password:"NAS-demo-only",website:"https://nas.example.test",notes:""},
    {id:7,name:"Notion 团队工作空间与长期项目资料",folder:"工作/共享账号",username:"docs@example.test",password:"Notion-demo-only",website:"https://notion.so",notes:"团队工作空间使用的账号。\n\n此处保留较长备注，用于查看右侧独立滚动时，左侧条目列表是否仍处于原来的位置。\n\n项目资料按季度整理，历史资料放在归档目录。\n\n共享信息由团队负责人维护。\n\n工作区内的页面与资料由使用者管理。\n\n完成后可继续选择左侧其他条目。"},
    {id:8,name:"Vercel",folder:"开发",username:"web@example.test",password:"Vercel-demo-only",website:"https://vercel.com",notes:""},
    {id:9,name:"Home Assistant",folder:"家庭/设备",username:"home",password:"Home-demo-only",website:"https://home.example.test",notes:""},
    {id:10,name:"邮件",folder:"生活",username:"pine@example.test",password:"Mail-demo-only",website:"https://mail.example.test",notes:""}
  ];
  let selected = initial === "list" ? null : 1;
  let editing = initial === "edit" || initial === "unsaved";
  let draft = editing ? {...entries[0]} : null;
  let pending = null;
  let revealed = false;
  let status = "";
  let error = "";
  const path = entry => entry.folder ? `${entry.folder}/${entry.name}` : entry.name;
  const entry = () => entries.find(item => item.id === selected);
  const dirty = () => editing && ["name","folder","password","username","website","notes"].some(key => draft[key] !== entry()[key]);
  const url = file => `${file}?lang=${language}`;
  let list, inspector, modal, search, folder;
  function shell() {
    const searchValue = search?.value ?? "";
    const folderValue = folder?.value ?? "";
    const listScroll = list?.scrollTop ?? 0;
    const panelScroll = inspector?.querySelector(".panel-body")?.scrollTop ?? 0;
    document.documentElement.lang = language;
    document.title = c.pageTitle;
    document.getElementById("root").innerHTML = `
      <main class="review">
        <div class="review-heading"><span class="eyebrow">${c.eyebrow}</span><h1>${c.heading}</h1></div>
        <p class="disclaimer">${c.disclaimer}</p>
        <nav class="review-nav" aria-label="${escape(c.heading)}">
          <a href="three-column/index.html?lang=${language}">${c.compareThreeColumn}</a>
          ${["list","detail","edit"].map((state,index) => `<a href="${url(`${state}.html`)}" data-stage="${state}">${c.stages[index]}</a>`).join("")}
          <a href="${url("unsaved.html")}" class="guard-link" data-stage="unsaved">${c.guardLink}</a>
        </nav>
        <div class="window-scroll"><section class="window" aria-label="Passpine">
          <header class="titlebar"><div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div><span class="app-name">Passpine</span><span class="vault">${c.vault}</span><span class="sync">${c.synced}</span></header>
          <div class="workspace">
            <section class="library" aria-label="${escape(c.listTitle)}">
              <header class="library-heading"><h2>${c.listTitle}</h2><span class="count" id="count"></span></header>
              <div class="list-tools"><div class="search"><svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><circle cx="8" cy="8" r="5.5"/><path d="m12 12 5 5"/></svg><input id="search" aria-label="${escape(c.search)}" placeholder="${escape(c.search)}"></div><select id="folder" aria-label="${escape(c.folderLabel)}"><option value="">${c.allFolders}</option>${[...new Set(entries.map(item => item.folder))].sort().map(folder => `<option>${escape(folder)}</option>`).join("")}</select></div>
              <div class="column-heading"><span>${c.nameColumn}</span><span>${c.folderColumn}</span><span></span></div>
              <div class="entry-list" id="entry-list"></div>
              <footer class="library-footer"><span id="list-footer"></span></footer>
            </section>
            <aside class="inspector" id="inspector" hidden></aside>
          </div>
          <div class="modal-layer" id="modal" hidden></div>
        </section></div>
        <div class="annotation" id="annotation"></div>
        <div class="review-footer"><p class="prototype-note">${c.interactionHint}</p><div class="language" aria-label="${escape(c.languageLabel)}"><a href="?lang=zh-CN" data-language="zh-CN" ${language === "zh-CN" ? 'aria-current="page"' : ""}>${c.languageChinese}</a><a href="?lang=en" data-language="en" ${language === "en" ? 'aria-current="page"' : ""}>${c.languageEnglish}</a></div></div>
        <p class="prototype-note">${c.prototypeOnly}</p>
      </main>`;
    list = document.getElementById("entry-list");
    inspector = document.getElementById("inspector");
    modal = document.getElementById("modal");
    search = document.getElementById("search");
    folder = document.getElementById("folder");
    search.value = searchValue;
    folder.value = folderValue;
    document.querySelectorAll("[data-language]").forEach(link => link.addEventListener("click", event => {
      event.preventDefault();
      if (link.dataset.language === language) return;
      const previous = c;
      language = link.dataset.language;
      c = globalThis.PASSPINE_WIREFRAME_COPY[language];
      if (status === previous.saved) status = c.saved;
      if (error) error = c[Object.keys(previous).find(key => previous[key] === error)] || error;
      const query = new URLSearchParams(location.search);
      query.set("lang", language);
      history.replaceState(null, "", `?${query}`);
      shell();
    }));
    bindListEvents();
    drawList(); drawPanel();
    list.scrollTop = listScroll;
    const panel = inspector.querySelector(".panel-body");
    if (panel) panel.scrollTop = panelScroll;
    if (pending) drawModal();
  }
  function drawList() {
    const scroll = list.scrollTop;
    const query = search.value.trim().toLowerCase();
    const visible = entries.filter(item => (!folder.value || item.folder === folder.value) && path(item).toLowerCase().includes(query));
    const matchCount = visible.length;
    const retained = selected && (editing || status === c.saved) && !visible.some(item => item.id === selected);
    if (retained) visible.unshift(entry());
    document.getElementById("count").textContent = retained ? text("retainedCount", {count:matchCount}) : visible.length === entries.length ? text("count", {count:entries.length}) : text("countFiltered", {visible:visible.length,total:entries.length});
    list.innerHTML = visible.length ? visible.map(item => `<button class="entry ${item.id === selected ? "selected" : ""}" data-entry="${item.id}" aria-label="${escape(text("entryLabel", item))}" aria-pressed="${item.id === selected}" title="${escape(path(item))}"><span class="entry-identity"><span class="monogram" aria-hidden="true">${escape(item.name.slice(0,2).toUpperCase())}</span><span class="entry-name">${escape(item.name)}</span></span><span class="entry-folder">${escape(item.folder)}</span><span class="arrow" aria-hidden="true">›</span></button>`).join("") : `<div class="empty">${c.empty}</div>`;
    list.scrollTop = scroll;
    list.querySelectorAll("[data-entry]").forEach(button => button.addEventListener("click", () => {
      const target = Number(button.dataset.entry);
      if (target !== selected) request({type:"select",target});
    }));
  }
  function updateNotes() {
    const state = pending ? "unsaved" : editing ? "edit" : selected ? "detail" : "list";
    const note = state === "unsaved" ? c.guardNote : state === "edit" ? c.editNote : state === "detail" ? c.detailNote : c.listNote;
    document.querySelectorAll("[data-stage]").forEach(link => {
      if (link.dataset.stage === state) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
    document.getElementById("annotation").textContent = note;
    document.getElementById("list-footer").textContent = selected ? editing ? c.footerEdit : c.footerDetail : c.footerList;
  }
  function drawPanel() {
    const item = entry();
    inspector.hidden = !item;
    inspector.classList.toggle("editor", editing);
    if (!item) { inspector.innerHTML = ""; updateNotes(); return; }
    inspector.setAttribute("aria-label", editing ? c.editorLabel : c.detailLabel);
    const heading = editing ? text("editing", item) : item.name;
    inspector.innerHTML = `<header class="inspector-heading"><div class="heading-text"><h2>${escape(heading)}</h2><p>${escape(path(item))}</p></div><div class="heading-actions">${editing ? `<span class="editor-state" id="edit-state">${dirty() ? c.changed : c.unchanged}</span>` : `<button id="edit">${c.edit}</button>`}<button class="quiet close" id="close" aria-label="${escape(c.close)}">×</button></div></header>${editing ? editorHTML() : detailsHTML(item)}`;
    document.getElementById("close").addEventListener("click", () => request({type:"close"}));
    if (editing) {
      inspector.querySelectorAll("[data-field]").forEach(input => input.addEventListener("input", () => {
        draft[input.dataset.field] = input.value;
        error = "";
        document.getElementById("edit-state").textContent = dirty() ? c.changed : c.unchanged;
        document.getElementById("save").disabled = !dirty();
        document.getElementById("form-error").textContent = "";
      }));
      document.getElementById("cancel").addEventListener("click", () => {editing=false;draft=null;status="";error="";drawPanel();});
      document.getElementById("save").addEventListener("click", () => save());
    } else {
      document.getElementById("edit").addEventListener("click", () => {editing=true;draft={...item};status="";drawPanel();});
      document.getElementById("reveal").addEventListener("click", () => {revealed=!revealed;drawPanel();});
    }
    updateNotes();
  }
  function detailsHTML(item) {
    const field = (label,value,secret=false) => `<div class="credential"><div class="field-title">${c[label]}</div><div class="credential-content"><span class="value ${secret ? "secret" : ""}">${escape(value)}</span>${secret ? `<button id="reveal">${revealed ? c.hide : c.show}</button>` : ""}<button disabled aria-label="${escape(text("copyDemo",{field:c[label]}))}">${c.copy}</button></div></div>`;
    return `<div class="panel-body">${field("password",revealed ? item.password : "••••••••••••",true)}${item.username ? field("username",item.username) : ""}${item.website ? field("website",item.website) : ""}${item.notes ? `<div class="notes"><div class="field-title">${c.notes}</div><p>${escape(item.notes)}</p></div>` : ""}${item.website ? `<button class="website-button" disabled aria-label="${escape(c.websiteDemo)}">${c.openWebsite}</button>` : ""}</div><footer class="panel-footer view-footer"><span class="saved-indicator" role="status">${status}</span></footer>`;
  }
  function editorHTML() {
    const row = (key,type="text") => `<label class="form-row"><span>${c[key]}</span>${key === "notes" ? `<textarea data-field="${key}">${escape(draft[key])}</textarea>` : `<input data-field="${key}" type="${type}" value="${escape(draft[key])}" autocomplete="off">`}</label>`;
    return `<div class="panel-body editor-form">${row("name")}${row("folder")}${row("password","password")}${row("username")}${row("website")}${row("notes")}<div class="form-error" id="form-error" role="alert">${error}</div></div><footer class="panel-footer"><span class="status"></span><button id="cancel">${c.cancel}</button><button class="primary" id="save" ${dirty() ? "" : "disabled"}>${c.save}</button></footer>`;
  }
  function request(action) {
    if (dirty()) {pending=action;drawModal();return;}
    execute(action);
  }
  function execute(action) {
    editing=false;draft=null;pending=null;revealed=false;status="";error="";
    selected = action.type === "select" ? action.target : null;
    modal.hidden=true;modal.innerHTML="";drawList();drawPanel();
  }
  function save(action=null) {
    draft.name=draft.name.trim();draft.folder=draft.folder.trim();
    error = !draft.name ? c.nameRequired : draft.name.includes("/") ? c.nameInvalid : /(^\/|\/$|\/\/)/.test(draft.folder) ? c.folderInvalid : entries.some(item => item.id !== selected && path(item) === path(draft)) ? c.duplicate : "";
    if (error) {pending=null;modal.hidden=true;modal.innerHTML="";drawPanel();return false;}
    Object.assign(entry(),draft);editing=false;draft=null;pending=null;status=c.saved;
    // Keep folder options current if this edit moves an entry to a new folder.
    if (!Array.from(folder.options).some(option => option.value === entry().folder)) folder.add(new Option(entry().folder,entry().folder));
    drawList();drawPanel();
    if (action) execute(action);
    return true;
  }
  function drawModal() {
    const item = entry();
    const message = pending.type === "select" ? text("guardSwitch",{name:item.name,target:entries.find(value => value.id === pending.target).name}) : text("guardClose",{name:item.name});
    modal.hidden=false;
    modal.innerHTML=`<section class="dialog" role="dialog" aria-modal="true" aria-labelledby="guard-title" aria-describedby="guard-message"><h2 id="guard-title">${c.guardTitle}</h2><p id="guard-message">${escape(message)}</p><div class="dialog-actions"><button class="primary" id="save-continue">${c.saveContinue}</button><button id="discard">${c.discard}</button><button id="keep-editing">${c.keepEditing}</button></div></section>`;
    document.getElementById("save-continue").addEventListener("click",()=>save(pending));
    document.getElementById("discard").addEventListener("click",()=>execute(pending));
    document.getElementById("keep-editing").addEventListener("click",closeModal);
    document.getElementById("keep-editing").focus();
    updateNotes();
  }
  function closeModal() {pending=null;modal.hidden=true;modal.innerHTML="";updateNotes();document.getElementById("save")?.focus();}
  function bindListEvents() {
    search.addEventListener("input", drawList);
    folder.addEventListener("change", drawList);
    modal.addEventListener("keydown", event => {
      if(event.key === "Escape") {event.preventDefault();closeModal();}
      if(event.key === "Tab") {
        const buttons=Array.from(modal.querySelectorAll("button"));
        const first=buttons[0],last=buttons[buttons.length-1];
        if(event.shiftKey && document.activeElement===first){event.preventDefault();last.focus();}
        else if(!event.shiftKey && document.activeElement===last){event.preventDefault();first.focus();}
      }
    });
    list.addEventListener("keydown",event=>{
      if(!["ArrowDown","ArrowUp"].includes(event.key))return;
      const buttons=Array.from(list.querySelectorAll("button"));
      const index=buttons.indexOf(document.activeElement);
      const next=buttons[index+(event.key==="ArrowDown"?1:-1)];
      if(next){event.preventDefault();next.focus();next.click();if(!pending)list.querySelector(`[data-entry="${selected}"]`)?.focus();}
    });
  }
  shell();
  if(initial === "unsaved") {draft.username="work@example.test";drawPanel();pending={type:"select",target:2};drawModal();}
})();
