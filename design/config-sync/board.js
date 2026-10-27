const c=window.configSyncCopy,s=document.body.dataset.state,m=document.querySelector('main');
const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text)n.textContent=text;if(cls)n.className=cls;return n};
const nav=el('nav');nav.append(el('h2',c.settings));for(const label of [c.general,c.security,c.vault,c.browser,c.title,c.cli,c.updates,c.about])nav.append(el('div',label,label===c.title?'active':''));document.body.prepend(nav);m.append(el('h1',c.title),el('p',c.description));
const card=el('section',null,'card');m.append(card);
const button=(label,cls)=>el('button',label,cls),actions=(...items)=>{const row=el('div',null,'actions');items.forEach(([text,cls])=>row.append(button(text,cls)));return row};
const field=(name,value,password=false,select=false)=>{const label=el('label',null,'field');label.append(el('span',name));const input=el(select?'select':'input');if(select)input.append(el('option',value));else{input.value=value;input.type=password?'password':'text'}label.append(input);return label};
const phrase=()=>{card.append(field(c.phrase,'example-long-sync-phrase',true),el('div',c.phraseHelp,'hint'))};
if(['connected','conflict'].includes(s)){
 const status=el('div',null,'status');status.append(el('span',null,'dot'),el('strong',c.saved),el('span','Cloudflare R2','badge'));card.append(status);
 const meta=el('div',null,'meta');meta.append(el('span','Personal account · passpine-config'),el('span','passpine/personal/'),el('span',c.lastSync));card.append(meta);
 if(s==='conflict'){card.append(el('hr'));const n=el('div',null,'notice');n.append(el('strong',c.conflict),el('p',c.conflictHelp));card.append(n);const values=el('div',null,'conflict-values');values.append(el('strong',c.conflictField));card.append(values,actions([c.keepLocal,'primary'],[c.useRemote,'']));}
 else card.append(actions([c.sync,'primary'],[c.edit,''],[c.disconnect,'ghost']));
}else{
 card.append(el('label',c.method));const segments=el('div',null,'segments');segments.append(button(c.oauth,['manual','error'].includes(s)?'':'selected'),button(c.manual,['manual','error'].includes(s)?'selected':''));card.append(segments);
 if(s==='waiting') {const wait=el('div',null,'waiting');wait.append(el('span','◌','spinner'),el('h3',c.waiting),el('div',c.waitingHelp,'hint'));card.append(wait,actions([c.cancel,'']));}
 else if(s==='bucket'){card.append(field(c.account,'Personal account',false,true));const row=el('div',null,'row');row.append(field(c.jurisdiction,c.defaultJurisdiction,false,true),field(c.bucket,'passpine-config',false,true));card.append(row,field(c.prefix,'passpine/personal/'),el('div',c.prefixHelp,'hint'));phrase();card.append(actions([c.select,'primary'],[c.cancel,'ghost']));}
 else if(['manual','error'].includes(s)){
 if(s==='error'){const n=el('div',null,'notice');n.append(el('strong',c.error),el('p',c.errorHelp));card.append(n)}
 card.append(el('div',c.manualHelp,'hint'),field(c.endpoint,'https://storage.example.com'));const row=el('div',null,'row');row.append(field(c.bucket,'passpine-config'),field(c.region,'auto'));card.append(row,field(c.prefix,'passpine/personal/'));
 const keys=el('div',null,'row');keys.append(field(c.key,'example-access-id'),field(c.secret,'example-secret-access-key',true));card.append(keys);const check=el('label',null,'check'),input=el('input');input.type='checkbox';input.checked=true;check.append(input,el('span',c.pathStyle));card.append(check);phrase();card.append(actions([c.save,'primary'],[c.test,''],[c.cancel,'ghost']));
 }else {card.append(el('p',c.oauthHelp),actions([c.connect,'primary']));}
}
const scope=el('section',null,'card');scope.append(el('h3',c.scope),el('p',c.scopeHelp),el('div',c.localHelp,'hint'));m.append(scope);
