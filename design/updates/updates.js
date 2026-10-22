(() => {
  const params = new URLSearchParams(location.search);
  let language = params.get('lang') === 'en' ? 'en' : 'zh-CN';
  let theme = params.get('theme') === 'dark' ? 'dark' : 'light';
  let state = document.body.dataset.state || 'latest';
  let autoDownload = state !== 'available';
  let modalOpen = state === 'confirm';
  let toastKey = '';
  if (state === 'confirm') state = 'ready';
  const root = document.getElementById('root');
  const c = () => globalThis.PASSPINE_UPDATE_DESIGN_COPY[language];
  const esc = value => String(value).replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
  const t = (key, values = {}) => esc(c()[key].replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? '')));
  const icon = name => `<svg viewBox="0 0 24 24" aria-hidden="true">${{
    back:'<path d="m14 6-6 6 6 6"/>',search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',system:'<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>',security:'<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z"/><path d="m8 12 3 3 5-6"/>',vault:'<rect x="3" y="5" width="18" height="15" rx="2"/><circle cx="12" cy="12.5" r="3"/><path d="M12 7v2m0 7v2m-5.5-5.5h2m7 0h2"/>',sync:'<path d="M20 7a9 9 0 0 0-15-2L3 7m0-4v4h4M4 17a9 9 0 0 0 15 2l2-2m0 4v-4h-4"/>',browser:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18m-14-2h.01M10 7h.01"/>',updates:'<path d="M12 3v12m-5-5 5 5 5-5M5 16v4h14v-4"/>',about:'<circle cx="12" cy="12" r="9"/><path d="M12 10v7m0-11h.01"/>',check:'<path d="m5 12 4 4L19 6"/>',spinner:'<path d="M21 12a9 9 0 1 1-5.7-8.4"/>',error:'<path d="m12 3 10 18H2zM12 9v5m0 3h.01"/>',moon:'<path d="M21 12.8A9 9 0 0 1 11.2 3a9 9 0 1 0 9.8 9.8Z"/>',external:'<path d="M14 3h7v7m0-7L10 14M11 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-6"/>',restart:'<path d="M3 11a9 9 0 1 1 2.7 7M3 4v7h7"/>'
  }[name] || ''}</svg>`;
  function statusMarkup() {
    const config = {
      checking:{title:'checkingTitle',detail:'checkingDetail',icon:'spinner'},
      latest:{title:'latestTitle',detail:'latestDetail',icon:'check'},
      available:{title:'availableTitle',detail:'availableDetail',icon:'updates',action:'download'},
      downloading:{title:'downloadTitle',detail:'downloadDetail',icon:'updates',progress:29,downloaded:'2.6 MB',total:'8.9 MB'},
      verifying:{title:'verifyingTitle',detail:'verifyingDetail',icon:'spinner'},
      ready:{title:'readyTitle',detail:'readyDetail',icon:'check',action:'restart'},
      failed:{title:'downloadFailedTitle',detail:'downloadFailedDetail',icon:'error',action:'retry'},
      'verify-failed':{title:'verificationFailedTitle',detail:'verificationFailedDetail',icon:'error',action:'retry'}
    }[state];
    const version = state === 'latest' ? '0.1.4' : '0.1.5';
    return `<section class="status-card ${config.icon === 'error' ? 'failure' : ''}" role="status" aria-live="polite">
      <div class="status-top"><span class="status-icon ${config.icon === 'spinner' ? 'spinner' : ''}">${icon(config.icon)}</span><div class="status-copy"><h2 class="status-title">${t(config.title,{version})}</h2><p class="status-detail">${t(config.detail,{version})}</p>${config.icon === 'error' ? `<p class="safe-detail">${t('safeDetail')}</p>` : ''}</div></div>
      ${config.progress ? `<div class="progress-area"><div class="progress-track" role="progressbar" aria-label="${t('progressLabel')}" aria-valuenow="${config.progress}" aria-valuemin="0" aria-valuemax="100"><div class="progress-fill" style="width:${config.progress}%"></div></div><p class="progress-caption">${t('downloadProgress',{downloaded:config.downloaded,total:config.total,percent:config.progress})}</p></div>` : ''}
      ${config.action || state === 'latest' ? `<div class="status-footer">${state === 'latest' ? `<span class="checked-at">${t('checkedAt')}</span>` : ''}${config.action ? `<button class="primary" data-action="${config.action}">${t(config.action)}</button>` : ''}</div>` : ''}
    </section>`;
  }
  function render() {
    const busy = ['checking','downloading','verifying'].includes(state);
    document.documentElement.lang = language;
    document.documentElement.dataset.theme = theme;
    document.title = c().pageTitle;
    root.innerHTML = `<div class="app"><header class="titlebar"><div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div><span class="brand">${t('app')}</span><div class="preview-tools" aria-label="${t('language')}"><button data-language="zh-CN" class="${language === 'zh-CN' ? 'selected' : ''}">${t('chinese')}</button><button data-language="en" class="${language === 'en' ? 'selected' : ''}">${t('english')}</button><button class="theme" data-action="theme" aria-label="${t('theme')}">${icon('moon')}</button></div></header><div class="workspace"><aside class="sidebar"><div class="back">${icon('back')}${t('back')}</div><h2>${t('settings')}</h2><div class="search">${icon('search')}${t('search')}</div><nav class="categories" aria-label="${t('settings')}">${['system','security','vault','browser','updates','about'].map(key=>`<div class="category ${key==='updates'?'current':''}" ${key==='updates'?'aria-current="page"':''}>${icon(key)}${t(key)}</div>`).join('')}</nav></aside><main class="page"><div class="page-inner"><div class="heading"><h1>${t('heading')}</h1><p class="description">${t('description')}</p></div><div class="version-row"><div><h2>${t('currentVersion')}</h2><p class="version-value">${t('version',{version:'0.1.4'})}</p></div><button class="check-button" data-action="check" ${busy?'disabled':''}>${icon('sync')}${t(state==='checking'?'checkingButton':'check')}</button></div>${statusMarkup()}<a class="release-row" href="https://github.com/knothhe/passpine/releases/tag/v${['latest','checking'].includes(state)?'0.1.4':'0.1.5'}" target="_blank" rel="noopener noreferrer" aria-label="${t('releaseLinkLabel',{version:['latest','checking'].includes(state)?'0.1.4':'0.1.5'})}"><span><strong>${t('notes')}</strong><span class="release-version">${t('releaseVersion',{version:['latest','checking'].includes(state)?'0.1.4':'0.1.5'})}</span></span>${icon('external')}</a><section class="preferences-card"><div class="setting-row"><div><h2 class="setting-title" id="auto-label">${t('autoDownload')}</h2><p class="setting-description" id="auto-description">${t('autoDescription')}</p></div><div class="segments" role="group" aria-labelledby="auto-label" aria-describedby="auto-description"><button class="${autoDownload?'selected':''}" aria-pressed="${autoDownload}" data-action="auto-on">${t('autoOn')}</button><button class="${!autoDownload?'selected':''}" aria-pressed="${!autoDownload}" data-action="auto-off">${t('autoOff')}</button></div></div><p class="behavior">${t('behavior')}</p></section><p class="install-hint">${icon('about')}<span>${t('installHint')}</span></p>${toastKey?`<p class="toast" role="status">${t(toastKey)}</p>`:''}</div></main></div></div>${modalOpen?`<div class="modal-layer"><section class="modal" role="dialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-detail"><div class="modal-icon">${icon('restart')}</div><h2 id="confirm-title">${t('confirmTitle')}</h2><p id="confirm-detail">${t('confirmDescription',{version:'0.1.5'})}</p><p class="modal-safety">${t('confirmSafety')}</p><div class="modal-actions"><button data-action="later">${t('later')}</button><button class="primary" data-action="install">${t('restart')}</button></div></section></div>`:''}`;
    root.querySelectorAll('[data-language]').forEach(button => button.addEventListener('click',() => {language=button.dataset.language;updateURL();render();}));
    root.querySelectorAll('[data-action]').forEach(button => button.addEventListener('click',() => action(button.dataset.action)));
    if(modalOpen) root.querySelector('[data-action="later"]').focus();
  }
  function updateURL(){params.set('lang',language);params.set('theme',theme);history.replaceState(null,'',`?${params}`);}
  function action(name) {
    if(name==='theme'){theme=theme==='light'?'dark':'light';updateURL();}
    if(name==='auto-on'){autoDownload=true;}
    if(name==='auto-off'){autoDownload=false;}
    if(name==='check'){state='checking';toastKey='';}
    if(name==='download'){state='downloading';toastKey='';}
    if(name==='retry'){state='downloading';toastKey='';}
    if(name==='restart'){modalOpen=true;}
    if(name==='later'){modalOpen=false;toastKey='toastLater';}
    if(name==='install'){modalOpen=false;toastKey='prototypeDone';}
    render();
  }
  document.addEventListener('keydown',event=>{if(event.key==='Escape'&&modalOpen){modalOpen=false;toastKey='toastLater';render();}if(event.key==='Tab'&&modalOpen){const buttons=[...root.querySelectorAll('.modal button')];if(event.shiftKey&&document.activeElement===buttons[0]){event.preventDefault();buttons.at(-1).focus();}else if(!event.shiftKey&&document.activeElement===buttons.at(-1)){event.preventDefault();buttons[0].focus();}}});
  render();
})();
