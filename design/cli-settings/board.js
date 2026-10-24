import {copy as c} from './constants/copy.js';
const state = document.body.dataset.state;
const main = document.querySelector('main');
const el = (tag, text, cls) => { const node=document.createElement(tag);node.textContent=text;if(cls)node.className=cls;return node; };
const nav=el('nav','');nav.append(el('h2',c.settings));for(const label of [c.general,c.security,c.vault,c.browser,c.title,c.updates,c.about])nav.append(el('div',label,label===c.title?'active':''));document.body.prepend(nav);
main.append(el('h1',c.title),el('p',c.description));
const section=el('section','','card');main.append(section);
if(state==='unsupported'){section.append(el('p',c.unsupported));}else{
const header=el('header',''),icon=el('div','>_','terminal'),info=el('div','','command'),badge=el('span',state==='installed'?c.installed:c.missing,'badge');info.append(el('strong','passpine-dev'),badge);const refresh=el('button',state==='busy'?'◌':'↻','refresh');refresh.setAttribute('aria-label',state==='busy'?c.working:c.refresh);refresh.title=c.refresh;header.append(icon,info,refresh);section.append(header,el('hr',''));
const row=el('div','','alias-row'),label=el('label',''),check=el('input','');check.type='checkbox';check.checked=['installed','busy','error'].includes(state);label.append(check,el('span',c.alias));row.append(label,el('span',state==='error'?c.conflict:state==='installed'?c.installed:c.missing,'badge'));section.append(row);
const actions=el('div','','actions');for(const [text,cls] of [[c.install,'primary'],[c.remove,'ghost']]){const button=el('button',text,cls);button.disabled=text===c.remove&&state!=='installed';actions.append(button);}section.append(actions);
if(state==='error')section.append(el('div',c.error,'error'));
const location=el('div','','location');location.append(el('span',c.location),el('code','/Users/demo/.local/bin'));main.append(location);
const details=el('details','');details.append(el('summary',c.help),el('p',c.path),el('p',c.removal));main.append(details);
}
