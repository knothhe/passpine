(() => {
  const params = new URLSearchParams(location.search);
  const language = params.get('lang') || (document.body.dataset.state === 'narrow' ? 'en' : 'zh-CN');
  const locale = language === 'en' ? 'en' : 'zh-CN';
  const c = globalThis.PASSPINE_COMMAND_PALETTE_COPY[locale];
  const state = document.body.dataset.state;
  const dark = params.get('theme') === 'dark' || state === 'narrow';
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  document.documentElement.lang = locale;
  document.title = c.canvasTitle;
  const escape = value => String(value).replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
  const format = (key, values) => c[key].replace(/\{(\w+)\}/g, (_, name) => values[name]);
  const paths = {
    search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',
    new:'<path d="M12 5v14M5 12h14"/>', edit:'<path d="m15 4 5 5M4 20l5-1L21 7a2 2 0 0 0-5-5L4 14Z"/>',
    username:'<circle cx="12" cy="8" r="4"/><path d="M4 21v-2a8 8 0 0 1 16 0v2"/>',
    password:'<circle cx="8" cy="9" r="5"/><path d="m12 13 8 8M16 17l3-3M18 19l3-3"/>',
    lock:'<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 1 1 8 0v3M12 14v3"/>',
    all:'<rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/>',
    drafts:'<path d="M14 3H5v18h14V8Z"/><path d="M14 3v5h5M8 12h8M8 16h5"/>', trash:'<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/>',
    general:'<path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="3"/><circle cx="15" cy="17" r="3"/>',
    security:'<path d="M12 3 4 6v6c0 5 8 9 8 9s8-4 8-9V6Z"/><path d="m8 12 3 3 5-6"/>',
    vault:'<path d="M3 7V5h6l2 3h10v12H3Z"/>', browser:'<circle cx="12" cy="12" r="9"/><ellipse cx="12" cy="12" rx="4" ry="9"/><path d="M3 12h18"/>',
    sync:'<path d="M8 19H6a5 5 0 0 1-1-10 7 7 0 0 1 13-1 5.5 5.5 0 0 1 0 11h-2M12 13v8m-3-3 3 3 3-3"/>',
    cli:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3m6 0h4"/>', updates:'<path d="M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5"/>', about:'<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10h.01"/>', close:'<path d="m6 6 12 12M6 18 18 6"/>',
  };
  const icon = name => `<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.all}</svg>`;
  Object.assign(paths, {'copy-password':paths.password,'copy-username':paths.username,'all-entries':paths.all,'open-vault':paths.vault,'settings':paths.general,'vault-settings':paths.vault,'config-sync':paths.sync});
  const shortcuts = {search:['⌘','F'],new:['⌘','N'],lock:['⌘','Shift','L'],settings:['⌘',',']};
  const keys = values => `<span class="chord">${values.map(key => `<kbd>${escape(key)}</kbd>`).join('')}</span>`;
  const ids = Object.keys(c.commands);
  let query = state === 'search' ? c.searchQuery : state === 'empty' ? c.emptyQuery : state === 'locked' ? c.lockedQuery : '';
  let selected = 0;
  let results = [];
  const unavailable = id => state === 'locked' && ['edit','copy-username','copy-password','lock'].includes(id);
  document.body.innerHTML = `<iframe id="context" tabindex="-1" aria-hidden="true" src="../client.html?lang=${locale}&theme=${dark?'dark':'light'}"></iframe><button id="reopen" hidden>${c.reopen}</button><div class="scrim" id="scrim"><section class="palette" role="dialog" aria-modal="true" aria-labelledby="palette-title"><header class="heading"><h1 id="palette-title">${c.title}</h1>${keys(['Ctrl','Shift','P'])}</header><div class="search">${icon('search')}<input id="query" role="combobox" aria-expanded="true" aria-controls="commands" aria-autocomplete="list" aria-label="${c.searchLabel}" placeholder="${c.placeholder}" autocomplete="off" spellcheck="false" value="${escape(query)}"><button class="close" id="close" aria-label="${c.close}">${icon('close')}</button></div><div class="results" id="commands" role="listbox" aria-label="${c.title}"></div><footer><span class="hint">${keys(['↑','↓'])}${c.choose}</span><span class="hint">${keys(['↵'])}${c.run}</span><span class="hint">${keys(['Esc'])}${c.dismiss}</span><span id="count"></span></footer></section></div><div id="notice" role="status"></div>`;
  const input = document.getElementById('query');
  const list = document.getElementById('commands');
  function render() {
    const terms = query.toLocaleLowerCase().trim().replace(/^>/, '').trim().split(/\s+/).filter(Boolean);
    results = ids.filter(id => terms.every(term => Object.values(globalThis.PASSPINE_COMMAND_PALETTE_COPY).flatMap(dictionary => dictionary.commands[id]).join(' ').toLocaleLowerCase().includes(term)));
    selected = Math.max(0, Math.min(selected, results.length-1));
    if (unavailable(results[selected])) selected = results.findIndex(id => !unavailable(id));
    document.getElementById('count').textContent = format(results.length === 1 ? 'countOne' : 'count',{count:results.length});
    if (!results.length) {
      input.removeAttribute('aria-activedescendant');
      list.innerHTML = `<div class="empty">${icon('search')}<h2>${c.emptyTitle}</h2><p>${c.emptyDescription}</p></div>`;
      return;
    }
    let lastGroup = '';
    list.innerHTML = results.map((id,index) => {
      const group = ids.indexOf(id) < 6 ? c.actions : c.navigation;
      const heading = !query.trim() && group !== lastGroup ? `<div class="group" role="presentation">${group}</div>` : '';
      lastGroup = group;
      const disabled = unavailable(id);
      return `${heading}<button type="button" role="option" tabindex="-1" id="command-${id}" class="command ${index===selected?'selected':''}" aria-selected="${index===selected}" aria-disabled="${disabled}" data-index="${index}">${icon(id)}<span class="labels"><span class="name">${c.commands[id][0]}</span><span class="description">${disabled?c.requiresUnlock:c.commands[id][1]}</span></span>${shortcuts[id]?keys(shortcuts[id]):index===selected&&!disabled?'<span class="return" aria-hidden="true">↵</span>':''}</button>`;
    }).join('');
    if(selected >= 0) input.setAttribute('aria-activedescendant',`command-${results[selected]}`); else input.removeAttribute('aria-activedescendant');
    list.querySelectorAll('.command').forEach(button => {
      button.onpointerdown = event => event.preventDefault();
      button.onclick = () => {selected=Number(button.dataset.index);run();};
      button.onpointermove = () => {if(!unavailable(results[Number(button.dataset.index)]) && selected !== Number(button.dataset.index)){selected=Number(button.dataset.index);render();}};
    });
  }
  function close() {document.getElementById('scrim').hidden=true;document.getElementById('reopen').hidden=false;document.getElementById('reopen').focus();}
  function open() {document.getElementById('scrim').hidden=false;document.getElementById('reopen').hidden=true;document.getElementById('notice').textContent='';query='';input.value='';selected=0;render();input.focus();}
  function run() {const id=results[selected];if(!id||unavailable(id))return;close();document.getElementById('notice').textContent=format('previewAction',{command:c.commands[id][0]});document.getElementById('reopen').hidden=true;}
  input.oninput=()=>{query=input.value;selected=0;render();};
  document.getElementById('close').onclick=close;
  document.getElementById('reopen').onclick=open;
  document.getElementById('scrim').onpointerdown=event=>{if(event.target.id==='scrim')close();};
  document.addEventListener('keydown',event=>{
    if(event.ctrlKey&&event.shiftKey&&event.key.toLowerCase()==='p'){event.preventDefault();open();return;}
    if(document.getElementById('scrim').hidden)return;
    if(event.isComposing)return;
    if(event.key==='Escape'){event.preventDefault();close();}
    if(['ArrowDown','ArrowUp'].includes(event.key)){event.preventDefault();if(results.length){for(let step=0;step<results.length;step++){selected=(selected+(event.key==='ArrowDown'?1:-1)+results.length)%results.length;if(!unavailable(results[selected]))break;}render();if(selected>=0)document.getElementById(`command-${results[selected]}`).scrollIntoView({block:'nearest'});input.focus();}}
    if(event.key==='Enter'&&document.activeElement===input){event.preventDefault();run();}
    if(event.key==='Tab'){const focusable=[input,document.getElementById('close')].filter(Boolean);const i=focusable.indexOf(document.activeElement);event.preventDefault();focusable[(i+(event.shiftKey?-1:1)+focusable.length)%focusable.length].focus();}
  });
  render();input.focus();
})();
