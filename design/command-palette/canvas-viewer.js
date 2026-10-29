// The artboard registration and viewer follow design/client-target's layout.
(() => {
  const lang = new URLSearchParams(location.search).get('lang') === 'en' ? 'en' : 'zh-CN';
  const copy = globalThis.PASSPINE_COMMAND_PALETTE_COPY[lang];
  document.documentElement.lang = lang;
  document.title = copy.canvasTitle;
  document.getElementById('title').textContent = copy.canvasTitle;
  document.getElementById('note').textContent = copy.note;
  for (const board of window.H2_DESIGN_CANVAS.boards) {
    const file = board.file.includes('lang=') ? board.file : `${board.file}?lang=${lang}`;
    const section = document.createElement('section');
    section.className = 'board';
    Object.assign(section.style, { left: `${board.x}px`, top: `${board.y}px`, width: `${board.w}px` });
    const link = document.createElement('a');
    link.href = file;
    link.textContent = copy[board.titleKey];
    link.target = '_blank';
    link.rel = 'noopener';
    const iframe = document.createElement('iframe');
    iframe.src = file;
    iframe.width = board.w;
    iframe.height = board.h;
    iframe.title = copy[board.titleKey];
    section.append(link, iframe);
    document.getElementById('canvas').append(section);
  }
})();
