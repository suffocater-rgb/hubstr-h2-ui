/* h2-ui SPA — dependency-free router + views */
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
      <a href="#/users" class="${r==='users'?'on':''}">Пользователи</a>
      <a href="#/config" class="${r==='config'?'on':''}">Сервер</a>
      <a href="#/logs" class="${r==='logs'?'on':''}">Логи</a>
      <a href="#" onclick="localStorage.removeItem('h2ui_token');location.reload();return false">Выход</a>
    </nav>
  </aside>
  <main id="main">${content}</main>`;
}

/* ---- views ---- */

function dashboard(){
  return api('/summary').then(s=>{
    layout(`
      <h2>Дашборд</h2>
      <div class="cards">
        <div class="card"><div class="k">Сервис</div><div class="v"><span class="dot ${s.status==='active'?'on':'off'}"></span>${esc(s.status)}</div></div>
        <div class="card"><div class="k">Пользователи</div><div class="v">${s.usersEnabled}/${s.usersTotal}</div></div>
        <div class="card"><div class="k">Онлайн сейчас</div><div class="v">${s.onlineTotal}</div></div>
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

function users(){
  return api('/users').then(list=>{
    const rows = list.map(u=>`
      <tr>
        <td>${u.id}</td>
        <td>${esc(u.username)}</td>
        <td><span class="badge ${u.enabled?'on':'off'}">${u.enabled?'вкл':'выкл'}</span></td>
        <td>↑ ${fmtBytes(u.up)} / ↓ ${fmtBytes(u.down)}</td>
        <td>${new Date(u.createdAt).toLocaleDateString()}</td>
        <td>
          <button class="sm" onclick="linkFor('${esc(u.username)}')">Ссылка</button>
          <button class="sm ghost" onclick="toggleUser('${esc(u.username)}',${!u.enabled})">${u.enabled?'Выкл':'Вкл'}</button>
          <button class="sm ghost" onclick="resetPw('${esc(u.username)}')">Пароль</button>
          <button class="sm ghost" onclick="kickUser('${esc(u.username)}')">Кик</button>
          <button class="sm danger" onclick="delUser('${esc(u.username)}')">Удалить</button>
        </td>
      </tr>`).join('');
    layout(`
      <div class="bar"><h2>Пользователи</h2>
        <div style="display:flex;gap:8px;width:340px">
          <input id="nu" placeholder="имя пользователя">
          <input id="np" placeholder="пароль">
          <button onclick="addUser()">Добавить</button>
        </div>
      </div>
      <table><thead><tr><th>#</th><th>Имя</th><th>Статус</th><th>Трафик</th><th>Создан</th><th></th></tr></thead>
      <tbody>${rows || '<tr><td colspan="6" class="muted">Пока пусто</td></tr>'}</tbody></table>
      <div id="linkbox" style="margin-top:14px"></div>`);
  });
}
window.addUser = ()=> api('/users',{method:'POST',body:JSON.stringify({username:nu.value,password:np.value})})
  .then(()=>{toast('Создан');users();}).catch(e=>toast(e.message,true));
window.delUser = n => confirm('Удалить '+n+'?') && api('/users/'+encodeURIComponent(n),{method:'DELETE'})
  .then(()=>{toast('Удалён');users();}).catch(e=>toast(e.message,true));
window.toggleUser = (n,en)=> api('/users/'+encodeURIComponent(n),{method:'PUT',body:JSON.stringify({enabled:en})})
  .then(()=>users()).catch(e=>toast(e.message,true));
window.resetPw = n=>{
  const p = prompt('Новый пароль для '+n+' (мин 4 символа)');
  if(!p) return;
  api('/users/'+encodeURIComponent(n),{method:'PUT',body:JSON.stringify({password:p})})
    .then(()=>toast('Пароль обновлён')).catch(e=>toast(e.message,true));
};
window.kickUser = n=> api('/users/'+encodeURIComponent(n)+'/kick',{method:'POST'})
  .then(()=>toast('Кикнут')).catch(e=>toast(e.message,true));
window.linkFor = n=> api('/link?user='+encodeURIComponent(n)).then(d=>{
  document.getElementById('linkbox').innerHTML = `
    <div class="card">
      <div class="k muted">Ссылка подключения для ${esc(n)}</div>
      <div class="copybox" style="margin-top:8px">
        <input readonly value="${esc(d.link)}" id="lnkin">
        <button class="sm" onclick="navigator.clipboard.writeText(document.getElementById('lnkin').value);toast('Скопировано')">Копировать</button>
      </div>
    </div>`;
}).catch(e=>toast(e.message,true));

function configView(){
  return Promise.all([api('/config'), api('/summary')]).then(([c,s])=>{
    layout(`
      <h2>Сервер</h2>
      <div class="card">
        <div class="row">
          <div><label>Порт (listen)</label><input id="cport" type="number" value="${c.port}"></div>
          <div><label>Masquerade URL</label><input id="cmasq" value="${esc(c.masquerade||'')}"></div>
        </div>
        <div class="row">
          <div><label>Obfs (salamander)</label>
            <select id="cobfs"><option value="1" ${c.obfs?'selected':''}>вкл</option><option value="0" ${!c.obfs?'selected':''}>выкл</option></select></div>
          <div><label>Obfs пароль</label><input id="cobfspw" value="${esc(c.obfsPassword||'')}"></div>
        </div>
        <button style="margin-top:14px" onclick="saveCfg()">Сохранить и перезапустить</button>
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

function logs(){
  return api('/logs?n=120').then(d=>{
    layout(`<h2>Логи hysteria-server</h2><pre>${esc((d.lines||[]).join('\n') || 'нет данных')}</pre>`);
  }).catch(e=>layout(`<h2>Логи</h2><div class="card">${esc(e.message)}</div>`));
}

function render(){
  if(!S.token){ loginView(); return; }
  ({dashboard, users, config:configView, logs}[S.route] || dashboard)();
}
render();
})();
