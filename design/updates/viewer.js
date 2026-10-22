(() => {
  const params = new URLSearchParams(location.search);
  let language = params.get('lang') === 'en' ? 'en' : 'zh-CN';
  const data = window.H2_DESIGN_CANVAS;
  const c = () => globalThis.PASSPINE_UPDATE_DESIGN_COPY[language];
  const esc = value => String(value).replace(/[&<>"']/g,char=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
  const t = key => esc(c()[key]);
  let zoom = .33, panX=26, panY=24, viewport, canvas, drag;
  function url(file){const u=new URL(file,location.href);if(!u.searchParams.has('lang'))u.searchParams.set('lang',language);return u.href;}
  function render(){
    document.documentElement.lang=language;
    document.title=c().canvasTitle;
    document.getElementById('root').innerHTML=`<header class="canvas-toolbar"><div class="canvas-heading"><h1>${t('canvasTitle')}</h1><p>${t('canvasIntro')}</p></div><nav class="canvas-tools" aria-label="${t('canvasControls')}"><button data-language="zh-CN" class="${language==='zh-CN'?'selected':''}">${t('chinese')}</button><button data-language="en" class="${language==='en'?'selected':''}">${t('english')}</button><button data-action="out" class="icon" aria-label="${t('zoomOut')}">−</button><output id="zoom">100%</output><button data-action="in" class="icon" aria-label="${t('zoomIn')}">+</button><button data-action="fit">${t('fit')}</button><button data-action="reset">${t('reset')}</button></nav></header><main id="viewport" tabindex="0" aria-label="${t('canvasLabel')}"><div id="canvas">${data.notes.map(note=>`<aside class="canvas-note" style="left:${note.x}px;top:${note.y}px;width:${note.w}px">${t(note.textKey)}</aside>`).join('')}${data.boards.map(board=>`<section class="board" style="left:${board.x}px;top:${board.y}px;width:${board.w}px;height:${board.h}px"><a href="${esc(url(board.file))}" target="_blank" rel="noreferrer">${t(board.titleKey)}<span>${t('open')} · ${board.w} × ${board.h}</span></a><iframe src="${esc(url(board.file))}" title="${t(board.titleKey)}" width="${board.w}" height="${board.h}" loading="lazy"></iframe></section>`).join('')}</div><p class="canvas-help">${t('canvasHint')}</p></main>`;
    viewport=document.getElementById('viewport');canvas=document.getElementById('canvas');
    document.querySelectorAll('[data-language]').forEach(button=>button.onclick=()=>{language=button.dataset.language;params.set('lang',language);history.replaceState(null,'',`?${params}`);render();});
    document.querySelectorAll('[data-action]').forEach(button=>button.onclick=()=>{const a=button.dataset.action;if(a==='fit'){zoom=Math.max(.1,Math.min(1,(viewport.clientWidth-52)/3380));panX=26;panY=25;}else if(a==='reset'){zoom=.33;panX=26;panY=24;}else scale(a==='in'?1.2:1/1.2);apply();});
    viewport.addEventListener('pointerdown',event=>{if(event.target.closest('a'))return;drag={x:event.clientX,y:event.clientY,panX,panY};viewport.setPointerCapture(event.pointerId);viewport.classList.add('dragging');});
    viewport.addEventListener('pointermove',event=>{if(!drag)return;panX=drag.panX+event.clientX-drag.x;panY=drag.panY+event.clientY-drag.y;apply();});
    viewport.addEventListener('pointerup',()=>{drag=null;viewport.classList.remove('dragging');});
    viewport.addEventListener('wheel',event=>{event.preventDefault();if(event.ctrlKey||event.metaKey){scale(Math.exp(-event.deltaY*.002),event.clientX,event.clientY-viewport.getBoundingClientRect().top);}else{panX-=event.deltaX;panY-=event.deltaY;}apply();},{passive:false});
    viewport.addEventListener('keydown',event=>{if(['+','=','-','0'].includes(event.key)){event.preventDefault();if(event.key==='0'){zoom=.33;panX=26;panY=24;}else scale(event.key==='-'?1/1.2:1.2);apply();}});
    apply();
  }
  function scale(factor,x=viewport.clientWidth/2,y=viewport.clientHeight/2){const next=Math.max(.12,Math.min(1.25,zoom*factor));panX=x-(x-panX)*next/zoom;panY=y-(y-panY)*next/zoom;zoom=next;}
  function apply(){canvas.style.transform=`translate(${panX}px,${panY}px) scale(${zoom})`;document.getElementById('zoom').textContent=`${Math.round(zoom*100)}%`;}
  render();
})();
