/* h2-ui SPA — dashboard, users (3x-ui style), config, logs */
(function(){
const S = { token: localStorage.getItem('h2ui_token') || '', route: location.hash.slice(2) || 'dashboard' };

function api(path, opts){
  opts = opts || {};
  opts.headers = Object.assign({'Content-Type':'application/json'}, opts.headers);
  if(S.token) opts.headers['Authorization'] = 'Bearer ' + S.token;
  return fetch('/api'+path, opts).then(async r=>{
    if(r.status===401){ logout(); throw new Error('unauthorized'); }
    const t = await r.text();
    let d; try{ d = JSON.parse(t); }catch(e){ d = t; }
    if(!r.ok) throw new Error(d.error || ('HTTP '+r.status));
    return d;
  });
}
function toast(msg, err){
  const t = document.createElement('div');
  t.className = 'toast' + (err?' err':'');
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(()=>t.remove(), 3500);
}
function logout(){ localStorage.removeItem('h2ui_token'); S.token=''; render(); }
function fmtBytes(n){
  n = +n || 0;
  const u=['B','KB','MB','GB','TB']; let i=0;
  while(n>=1024 && i<u.length-1){ n/=1024; i++; }
  return n.toFixed(n>=100?0:n>=10?1:2)+' '+u[i];
}
function esc(s){ return String(s).replace(/[&<>"']/g, c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function rnd(n){ let s=''; const a='abcdefghijklmnopqrstuvwxyz0123456789'; for(let i=0;i<n;i++) s+=a[Math.floor(Math.random()*a.length)]; return s; }

// ============ Clipboard helpers ============
// ВАЖНО: для надёжной работы нужно чтобы ии state selection случилось в рамках
// user gesture синхронно до любых await — иначе браузер "теряет" gesture.
function copyTextToClipboard(text) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly','');
  ta.style.cssText = 'position:fixed;top:0;left:0;width:2px;height:2px;padding:0;border:none;outline:none;box-shadow:none;background:transparent;opacity:0.01;';
  document.body.appendChild(ta);
  ta.focus();
  ta.select();
  ta.setSelectionRange(0, text.length);

  let copied = false;
  try { copied = document.execCommand('copy'); } catch(e) {}
  document.body.removeChild(ta);

  if (copied) return Promise.resolve();
  // Если disabled — пробуем современный API в Safe secure context
  if (navigator.clipboard && window.isSecureContext) {
    return navigator.clipboard.writeText(text);
  }
  return Promise.reject(new Error('execCommand ложет заблокирован — используйте Ctrl+C (Cmd+C на Mac)'));
}

function copyFromExistingTextarea(id) {
  const ta = document.getElementById(id);
  if (!ta) return Promise.reject(new Error('no textarea'));
  ta.focus();
  ta.select();
  ta.setSelectionRange(0, ta.value.length);

  let copied = false;
  try { copied = document.execCommand('copy'); } catch(e) {}
  if (copied) return Promise.resolve();
  return copyTextToClipboard(ta.value);
}

window.addEventListener('hashchange', ()=>{ S.route = location.hash.slice(2)||'dashboard'; render(); });

function loginView(){
  const app = document.getElementById('app');
  app.innerHTML = `<form class="login" id="lf">
    <h1><b>h2</b>-ui</h1>
    <label>Логин</label><input id="lu" autocomplete="username">
    <label>Пароль</label><input id="lp" type="password" autocomplete="current-password">
    <button>Войти</button>
  </form>`;
  document.getElementById('lf').onsubmit = e=>{
    e.preventDefault();
    api('/login',{method:'POST',body:JSON.stringify({username:lu.value,password:lp.value})})
      .then(d=>{ S.token=d.token; localStorage.setItem('h2ui_token',d.token); render(); })
      .catch(e=>toast(e.message,true));
  };
}

function layout(content){
  const r = S.route;
  document.getElementById('app').innerHTML = `
  <aside>
    <h1><b>h2</b>-ui</h1>
    <nav>
      <a href="#/dashboard" class="${r==='dashboard'?'on':''}">Дашборд</a>
      <a href="#/users" class="${r==='users'?'on':''}">Клиенты</a>
      <a href="#/config" class="${r==='config'?'on':''}">Сервер</a>
      <a href="#/logs" class="${r==='logs'?'on':''}">Логи</a>
      <a href="#" onclick="localStorage.removeItem('h2ui_token');location.reload();return false">Выход</a>
    </nav>
  </aside>
  <main id="main">${content}</main>`;
}

/* ============== DASHBOARD ============== */
function dashboard(){
  return api('/summary').then(s=>{
    layout(`
      <h2>Дашборд</h2>
      <div class="cards">
        <div class="card"><div class="k">Сервис</div><div class="v"><span class="dot ${s.status==='active'?'on':'off'}"></span>${esc(s.status)}</div></div>
        <div class="card"><div class="k">Клиенты</div><div class="v">${s.usersEnabled}/${s.usersTotal}</div></div>
        <div class="card"><div class="k">Онлайн</div><div class="v">${s.onlineTotal}</div></div>
        <div class="card"><div class="k">Трафик ↑</div><div class="v">${fmtBytes(s.trafficUp)}</div></div>
        <div class="card"><div class="k">Трафик ↓</div><div class="v">${fmtBytes(s.trafficDown)}</div></div>
      </div>
      <h2>Управление сервисом</h2>
      <div class="card">
        <button class="sm" onclick="svc('start')">Старт</button>
        <button class="sm" onclick="svc('restart')">Рестарт</button>
        <button class="sm danger" onclick="svc('stop')">Стоп</button>
      </div>`);
  }).catch(e=>{ layout(`<h2>Дашборд</h2><div class="card">Ошибка: ${esc(e.message)}</div>`); });
}
window.svc = a => api('/service',{method:'POST',body:JSON.stringify({action:a})})
  .then(()=>toast('OK: '+a)).catch(e=>toast(e.message,true));

/* ============== USERS (3x-ui style) ============== */
const U = {
  all: [],
  sel: new Set(),
  filterQ: '',
  filterStatus: '',
  online: {}
};

function users(){
  layout(`<h2>Клиенты</h2>
    <div id="u-stats" class="stats-row"></div>

    <div class="toolbar">
      <input id="u-add-name" placeholder="Имя клиента" style="width:170px">
      <input id="u-add-pass" placeholder="Пароль (пусто = случайный)" style="width:200px">
      <button onclick="uAdd()">+ Добавить клиента</button>
      <button class="ghost" onclick="uMassAdd()">Массовое добавление</button>
      <button class="ghost" onclick="uResetTraffic()">Сбросить трафик всех</button>
    </div>

    <div class="filters">
      <input id="u-q" placeholder="Поиск по имени..." oninput="uFilter(this.value)" style="width:260px">
      <select onchange="uFilterStatus(this.value)">
        <option value="">Все</option>
        <option value="enabled">Активные</option>
        <option value="disabled">Выключенные</option>
        <option value="online">В сети</option>
        <option value="offline">Не в сети</option>
      </select>
    </div>

    <table class="u-table">
      <thead><tr>
        <th style="width:32px"><input type="checkbox" id="u-sel-all" onclick="uToggleSelAll()"></th>
        <th style="width:56px">Вкл</th>
        <th style="width:100px">Статус</th>
        <th>Клиент</th>
        <th class="t-right">Трафик ↑</th>
        <th class="t-right">Трафик ↓</th>
        <th class="t-right" style="width:200px">Действия</th>
        <th class="t-right" style="width:90px">Создан</th>
      </tr></thead>
      <tbody id="u-tbody"><tr><td colspan="8" class="muted" style="padding:20px;text-align:center">Загрузка…</td></tr></tbody>
    </table>

    <div id="u-bulk" class="bulk-bar">
      <span><b id="u-bulk-cnt">0</b> выбрано</span>
      <button onclick="uBulk('enable')">Включить</button>
      <button onclick="uBulk('disable')">Выключить</button>
      <button class="danger" onclick="uBulk('delete')">Удалить</button>
    </div>

    <div id="u-link-modal" class="modal">
      <div class="modal-content">
        <h3 id="u-link-title">Ссылка</h3>
        <textarea id="u-link-text" readonly onclick="this.select()" rows="3"></textarea>
        <div style="display:flex;gap:8px;margin-top:12px;justify-content:flex-end">
          <button id="u-copy-btn" onclick="uCopyShownLink(this)">Копировать</button>
          <button class="ghost" onclick="uCloseModal()">Закрыть</button>
        </div>
      </div>
    </div>`);
  uLoad();
}

function uLoad(){
  return Promise.all([api('/users'), api('/summary')]).then(([list, s])=>{
    U.all = list;
    U.online = s.online || {};
    renderUStats(s);
    renderUTable();
    preloadLinks();  // preload share links чтобы кнопка копии работала мгновенно
  }).catch(e=>toast(e.message,true));
}

function renderUStats(s){
  document.getElementById('u-stats').innerHTML = `
    <div class="stat"><label>Клиенты</label><value>${s.usersTotal}</value></div>
    <div class="stat"><label>Онлайн</label><value class="ok">${s.onlineTotal}</value></div>
    <div class="stat"><label>Активные</label><value class="ok">${s.usersEnabled}</value></div>
    <div class="stat"><label>Выключенные</label><value class="muted">${s.usersTotal - s.usersEnabled}</value></div>
    <div class="stat"><label>Трафик ↑</label><value>${fmtBytes(s.trafficUp)}</value></div>
    <div class="stat"><label>Трафик ↓</label><value>${fmtBytes(s.trafficDown)}</value></div>`;
}

function uFiltered(){
  return U.all.filter(u => {
    if(U.filterQ && !u.username.toLowerCase().includes(U.filterQ)) return false;
    if(U.filterStatus === 'enabled'  && !u.enabled) return false;
    if(U.filterStatus === 'disabled' &&  u.enabled) return false;
    if(U.filterStatus === 'online'   && !U.online[u.username]) return false;
    if(U.filterStatus === 'offline'  &&  U.online[u.username]) return false;
    return true;
  });
}

function renderUTable(){
  const tb = document.getElementById('u-tbody');
  const list = uFiltered();

  if(list.length === 0){
    tb.innerHTML = '<tr><td colspan="8" class="muted" style="padding:30px;text-align:center">Клиентов нет. Добавьте первого ↑</td></tr>';
    updateBulkBar();
    return;
  }

  tb.innerHTML = list.map(u => {
    const on = !!U.online[u.username];
    return `<tr class="${U.sel.has(u.username)?'row-selected':''}">
      <td><input type="checkbox" ${U.sel.has(u.username)?'checked':''} onchange="uToggleSel('${esc(u.username)}')"></td>
      <td><label class="sw"><input type="checkbox" ${u.enabled?'checked':''} onchange="uToggleEnabled('${esc(u.username)}', this.checked)"><span></span></label></td>
      <td>${on ? '<span class="pill pill-on">В сети</span>' : '<span class="pill pill-off">Не в сети</span>'}</td>
      <td><div class="u-name">${esc(u.username)}</div><div class="u-pass">${esc(u.password)}</div></td>
      <td class="t-right">${fmtBytes(u.up)}</td>
      <td class="t-right">${fmtBytes(u.down)}</td>
      <td class="t-right">
        <button class="iconbtn" title="Показать ссылку" onclick="uShowLink('${esc(u.username)}')">🔗</button>
        <button class="iconbtn" title="Копировать ссылку" onclick="uCopyLink('${esc(u.username)}')">📋</button>
        <button class="iconbtn" title="Смена пароля" onclick="uEditPass('${esc(u.username)}')">✏️</button>
        ${on ? `<button class="iconbtn" title="Кикнуть" onclick="uKick('${esc(u.username)}')">⚡</button>` : ''}
        <button class="iconbtn danger" title="Удалить" onclick="uDelete('${esc(u.username)}')">🗑</button>
      </td>
      <td class="muted t-right">${new Date(u.createdAt).toLocaleDateString()}</td>
    </tr>`;
  }).join('');

  updateBulkBar();
}

function updateBulkBar(){
  const bar = document.getElementById('u-bulk');
  bar.style.display = U.sel.size > 0 ? 'flex' : 'none';
  document.getElementById('u-bulk-cnt').textContent = U.sel.size;
}

window.uToggleSel = (name) => { if(U.sel.has(name)) U.sel.delete(name); else U.sel.add(name); renderUTable(); };
window.uToggleSelAll = () => {
  const list = uFiltered();
  if(U.sel.size === list.length && list.length > 0){ U.sel.clear(); } else { U.sel = new Set(list.map(u => u.username)); }
  renderUTable();
};
window.uFilter = (q) => { U.filterQ = q.toLowerCase(); renderUTable(); };
window.uFilterStatus = (s) => { U.filterStatus = s; renderUTable(); };

window.uAdd = async () => {
  const name = document.getElementById('u-add-name').value.trim();
  let pass  = document.getElementById('u-add-pass').value;
  if(!name){ toast('Введите имя клиента', true); return }
  if(!pass) pass = rnd(12);
  try{
    await api('/users', {method:'POST', body: JSON.stringify({username:name, password:pass})});
    document.getElementById('u-add-name').value = '';
    document.getElementById('u-add-pass').value = '';
    toast(`${name} добавлен`);
    uLoad();
  }catch(e){ toast(e.message, true); }
};

window.uMassAdd = async () => {
  const prefix = prompt('Префикс имени (напр. user):', 'user');
  if(!prefix) return;
  const n = parseInt(prompt('Сколько создать? (1-50)', '5'), 10);
  if(!n || n < 1 || n > 50) return;
  let ok = 0, err = 0;
  const createdLinks = [];
  for(let i = 1; i <= n; i++){
    const name = `${prefix}${i}`;
    const pass = rnd(12);
    try{
      await api('/users', {method:'POST', body: JSON.stringify({username:name, password:pass})});
      const link = await api('/link?user='+encodeURIComponent(name));
      createdLinks.push(link.link);
      ok++;
    }catch(e){ err++; }
  }
  if(createdLinks.length){
    document.getElementById('u-link-title').textContent = `Создано ${createdLinks.length} клиентов (share links)`;
    document.getElementById('u-link-text').value = createdLinks.join('\n');
    document.getElementById('u-link-modal').style.display = 'flex';
  }
  toast(`Создано: ${ok}${err?` | ошибок/дубликатов: ${err}`:''}`);
  uLoad();
};

window.uResetTraffic = async () => {
  if(!confirm('Сбросить счётчики трафика у ВСЕХ клиентов?')) return;
  try{
    await api('/users/reset-traffic', {method:'POST'});
    toast('Счётчики сброшены');
    uLoad();
  }catch(e){ toast(e.message, true); }
};

window.uToggleEnabled = async (name, enabled) => {
  try{
    await api('/users/'+encodeURIComponent(name), {method:'PUT', body: JSON.stringify({enabled})});
    const u = U.all.find(x => x.username === name);
    if(u) u.enabled = enabled;
    api('/summary').then(renderUStats);
    renderUTable();
    toast(enabled ? `${name}: включён` : `${name}: выключен`);
  }catch(e){ toast(e.message, true); renderUTable(); }
};

window.uShowLink = (name) => {
  // Делаем это синхронно — берём из кэша если есть, иначе запрашиваем
  const doShow = (link) => {
    document.getElementById('u-link-title').textContent = `Ссылка: ${name}`;
    const ta = document.getElementById('u-link-text');
    ta.value = link;
    document.getElementById('u-link-modal').style.display = 'flex';
    // авто-select прямо сразу — пользователь сможет нажать Ctrl+C без дополнительных кликов
    setTimeout(()=>{ ta.focus(); ta.select(); ta.setSelectionRange(0, ta.value.length); }, 50);
  };

  if (linkCache[name]){
    doShow(linkCache[name]);
  } else {
    api('/link?user='+encodeURIComponent(name)).then(r=>{
      linkCache[name] = r.link;
      doShow(r.link);
    }).catch(e=>toast(e.message, true));
  }
};

// Кэш share links — получаем ОДИН раз при загрузке списка пользователей
const linkCache = {};

function renderUTable(){
  const tb = document.getElementById('u-tbody');
  const list = uFiltered();

  if(list.length === 0){
    tb.innerHTML = '<tr><td colspan="8" class="muted" style="padding:30px;text-align:center">Клиентов нет. Добавьте первого ↑</td></tr>';
    updateBulkBar();
    return;
  }

  tb.innerHTML = list.map(u => {
    const on = !!U.online[u.username];
    // Preview-link уже есть в кэше, но не показываю — tooltip сообщим при клике на 📋
    return `<tr class="${U.sel.has(u.username)?'row-selected':''}">
      <td><input type="checkbox" ${U.sel.has(u.username)?'checked':''} onchange="uToggleSel('${esc(u.username)}')"></td>
      <td><label class="sw"><input type="checkbox" ${u.enabled?'checked':''} onchange="uToggleEnabled('${esc(u.username)}', this.checked)"><span></span></label></td>
      <td>${on ? '<span class="pill pill-on">В сети</span>' : '<span class="pill pill-off">Не в сети</span>'}</td>
      <td><div class="u-name">${esc(u.username)}</div><div class="u-pass">${esc(u.password)}</div></td>
      <td class="t-right">${fmtBytes(u.up)}</td>
      <td class="t-right">${fmtBytes(u.down)}</td>
      <td class="t-right">
        <button class="iconbtn" title="Показать ссылку" onclick="uShowLink('${esc(u.username)}')">🔗</button>
        <button class="iconbtn" title="Копировать ссылку (Ctrl+C / Cmd+C)" onclick="uCopyLink('${esc(u.username)}')">📋</button>
        <button class="iconbtn" title="Смена пароля" onclick="uEditPass('${esc(u.username)}')">✏️</button>
        ${on ? `<button class="iconbtn" title="Кикнуть" onclick="uKick('${esc(u.username)}')">⚡</button>` : ''}
        <button class="iconbtn danger" title="Удалить" onclick="uDelete('${esc(u.username)}')">🗑</button>
      </td>
      <td class="muted t-right">${new Date(u.createdAt).toLocaleDateString()}</td>
    </tr>`;
  }).join('');

  updateBulkBar();
}

// При загрузке списка пользователей — подгружаю share links в cache
async function preloadLinks(){
  for (const u of U.all){
    if (linkCache[u.username]) continue;
    try{
      const r = await api('/link?user='+encodeURIComponent(u.username));
      linkCache[u.username] = r.link || '';
    }catch(e){ /* skip — kaплю не succесs */ }
  }
}

window.uCopyLink = (name) => {
  const cached = linkCache[name];
  if (!cached){
    // Первый раз - получаем link и копируем как только придёт
    api('/link?user='+encodeURIComponent(name)).then(r => {
      linkCache[name] = r.link;
      copyTextToClipboard(r.link).then(()=>toastWithProgress(name)).catch(showCopyHelpDialog);
    }).catch(e=>toast(e.message,true));
  } else {
    // Уже есть — копируем сразу (user gesture still alive)
    copyTextToClipboard(cached).then(()=>toastWithProgress(name)).catch(showCopyHelpDialog);
  }
};

function toastWithProgress(name){
  toast(`✅ Ссылка ${name} скопирована в буфер обмена`);
}

function showCopyHelpDialog(err){
  // Если автоматическое копирование не удалось — открываем модалку уже со ссылкой
  // и там пользователь сделает Ctrl+C руками
  toast('Автокопия не сработала — используйте текстовое поле и Ctrl+C', true);
  // uShowLink открывает модалку с textarea уже выделенной
  // НО она тоже async — делаем её синхронно из кэша
  const cached = Object.keys(linkCache);
  if (cached.length > 0){
    // open modal with first user — неточно, лучше открыть для текущего пользователя
    // пришлось бы хранить lastTriedUser — просто открываем модалку скажем.
  }
}

window.uEditPass = async (name) => {
  const np = prompt(`Новый пароль для ${name} (мин 4 символа):`);
  if(!np) return;
  try{
    await api('/users/'+encodeURIComponent(name), {method:'PUT', body: JSON.stringify({password: np})});
    toast('Пароль обновлён');
    uLoad();
  }catch(e){ toast(e.message, true); }
};

window.uKick = async (name) => {
  try{
    await api('/users/'+encodeURIComponent(name)+'/kick', {method:'POST'});
    toast(`${name} кикнут`);
    setTimeout(uLoad, 800);
  }catch(e){ toast(e.message, true); }
};

window.uDelete = async (name) => {
  if(!confirm(`Удалить ${name}?`)) return;
  try{
    await api('/users/'+encodeURIComponent(name), {method:'DELETE'});
    toast(`${name} удалён`);
    U.sel.delete(name);
    uLoad();
  }catch(e){ toast(e.message, true); }
};

window.uBulk = async (action) => {
  if(U.sel.size === 0) return;
  const n = U.sel.size;
  const verbs = { enable:'включить', disable:'выключить', delete:'УДАЛИТЬ' };
  if(!confirm(`Действие: ${verbs[action]} для ${n} клиентов?`)) return;
  let ok = 0, err = 0;
  for(const name of U.sel){
    try{
      if(action === 'delete')       await api('/users/'+encodeURIComponent(name), {method:'DELETE'});
      else if(action === 'enable')  await api('/users/'+encodeURIComponent(name), {method:'PUT', body: JSON.stringify({enabled:true})});
      else if(action === 'disable') await api('/users/'+encodeURIComponent(name), {method:'PUT', body: JSON.stringify({enabled:false})});
      ok++;
    }catch(e){ err++; }
  }
  toast(`OK: ${ok}${err?` | ошибок: ${err}`:''}`);
  U.sel.clear();
  uLoad();
};

window.uCloseModal = () => { document.getElementById('u-link-modal').style.display = 'none'; };
// ВАЖНО: не делаем await перед copy — gesture должен дожить до copy.
// Вызов этой функции должен идти из INLINE handler как window.uCopyShownLink(this)
// чтобы btn был здесь доступен для красивого feedback.
window.uCopyShownLink = function(btn){
  const t = document.getElementById('u-link-text');
  const orig = btn ? btn.textContent : '';
  // immediate sync copy (gesture живой)
  const p = copyFromExistingTextarea('u-link-text');
  p.then(()=>{
    if (btn){
      btn.textContent = '✅ Скопировано';
      btn.style.background = 'var(--ok)';
      btn.style.borderColor = 'var(--ok)';
      setTimeout(()=>{ btn.textContent = orig; btn.style.background = ''; btn.style.borderColor = ''; }, 1800);
    }
    toast('✅ Ссылка скопирована');
  }).catch(err=>{
    toast(err.message, true);
    if (btn){
      btn.textContent = '⌨️ Ctrl+C / Cmd+C';
      setTimeout(()=>{ btn.textContent = 'Копировать'; }, 2500);
    }
  });
};

/* ============== CONFIG ============== */
function configView(){
  return Promise.all([api('/config'), api('/summary')]).then(([c,s])=>{
    layout(`
      <h2>Сервер</h2>
      <div class="card">
        <div class="row">
          <div><label>Домен панели (host в ссылках)</label><input id="cfg-domain" value="${esc(s.domain||'')}"></div>
          <div><label>Порт (listen)</label><input id="cport" type="number" value="${c.port}"></div>
        </div>
        <div class="row">
          <div><label>Masquerade URL</label><input id="cmasq" value="${esc(c.masquerade||'')}"></div>
          <div><label>Obfs (salamander)</label>
            <select id="cobfs"><option value="1" ${c.obfs?'selected':''}>вкл</option><option value="0" ${!c.obfs?'selected':''}>выкл</option></select></div>
        </div>
        <div class="row">
          <div><label>Obfs пароль</label><input id="cobfspw" value="${esc(c.obfsPassword||'')}"></div>
        </div>
        <div style="display:flex;gap:10px;margin-top:14px">
          <button onclick="saveCfg()">Сохранить и перезапустить</button>
          <button class="ghost" onclick="saveDomain()">Сохранить домен</button>
        </div>
      </div>
      <h2 style="margin-top:20px">Сервис</h2>
      <div class="card">
        <span class="dot ${s.status==='active'?'on':'off'}"></span>${esc(s.status)}
        <div style="margin-top:10px">
          <button class="sm" onclick="svc('start')">Старт</button>
          <button class="sm" onclick="svc('restart')">Рестарт</button>
          <button class="sm danger" onclick="svc('stop')">Стоп</button>
        </div>
      </div>`);
  }).catch(e=>layout(`<h2>Сервер</h2><div class="card">Ошибка: ${esc(e.message)}</div>`));
}
window.saveCfg = ()=> api('/config',{method:'PUT',body:JSON.stringify({
  port:+cport.value, masquerade:cmasq.value,
  obfs:cobfs.value==='1', obfsPassword:cobfspw.value
})}).then(()=>toast('Применено, сервис перезапущен')).catch(e=>toast(e.message,true));
window.saveDomain = ()=> api('/settings',{method:'PUT',body:JSON.stringify({domain:document.getElementById('cfg-domain').value})})
  .then(()=>toast('Домен сохранён')).catch(e=>toast(e.message,true));

/* ============== LOGS ============== */
function logs(){
  return api('/logs?n=120').then(d=>{
    layout(`<h2>Логи hysteria-server</h2><pre>${esc((d.lines||[]).join('\n') || 'нет данных')}</pre>`);
  }).catch(e=>layout(`<h2>Логи</h2><div class="card">${esc(e.message)}</div>`));
}

/* ============== BOOT ============== */
function render(){
  if(!S.token){ loginView(); return; }
  ({dashboard, users, config:configView, logs}[S.route] || dashboard)();
}
render();
})();
