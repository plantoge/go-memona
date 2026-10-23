'use strict';

(() => {
  const $ = (id) => document.getElementById(id);
  let csrf = '';
  let generation = 0;
  let view = 'notes';
  let requestID = 0;
  let detailID = 0;
  let selectedNote = null;
  let searchTimer;
  const titles = {
    notes: ['Semua memori', 'Temukan konteks, keputusan, dan pengetahuan lintas proyek.'],
    skills: ['Pustaka skill', 'Pengetahuan yang dapat digunakan ulang, dalam satu tempat.'],
    keys: ['Kunci API', 'Kelola akses integrasi tanpa membagikan kata sandi administrator.'],
    activity: ['Aktivitas', 'Pantau jejak perubahan di ruang memori Anda.']
  };

  function text(tag, value, className) {
    const node = document.createElement(tag);
    node.textContent = value == null ? '' : String(value);
    if (className) node.className = className;
    return node;
  }
  function status(message) { $('status').textContent = message; }
  function clearError() { $('error').hidden = true; $('error').textContent = ''; }
  function showError(error) {
    if (error.stale) return;
    $('error').textContent = error.message || 'Terjadi kesalahan. Silakan coba lagi.';
    $('error').hidden = false;
  }
  function date(value) {
    if (!value) return '—';
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? 'Tanggal tidak valid' : parsed.toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' });
  }
  function empty(container, message) { container.replaceChildren(text('p', message, 'empty-state')); }
  function clearSecret() { $('new-token').value = ''; $('secret-panel').hidden = true; }
  function signedOut() {
    generation++;
    requestID++;
    detailID++;
    csrf = '';
    selectedNote = null;
    clearTimeout(searchTimer);
    clearSecret();
    if ($('detail-dialog').open) $('detail-dialog').close();
    ['notes-list', 'keys-list', 'activity-list', 'note-content', 'note-history', 'detail-meta'].forEach((id) => $(id).replaceChildren());
    $('detail-title').textContent = 'Detail memori';
    $('detail-status').textContent = '';
    $('detail-error').textContent = '';
    $('detail-body').hidden = true;
    $('key-form').reset();
    $('filters').reset();
    $('password').value = '';
    $('project-filter').replaceChildren(new Option('Semua proyek', ''));
    ['note-count', 'skill-count', 'project-count'].forEach((id) => { $(id).textContent = '—'; });
    $('dashboard').hidden = true;
    $('login').hidden = false;
    $('password').focus();
  }
  async function api(path, options = {}) {
    const currentGeneration = generation;
    const headers = { Accept: 'application/json' };
    if (options.body !== undefined) headers['Content-Type'] = 'application/json';
    if (options.method && options.method !== 'GET' && csrf) headers['X-CSRF-Token'] = csrf;
    let response;
    try {
      response = await fetch(path, { credentials: 'same-origin', cache: 'no-store', ...options, headers, body: options.body === undefined ? undefined : JSON.stringify(options.body) });
    } catch (_) {
      if (currentGeneration !== generation) throw Object.assign(new Error('Respons lama diabaikan.'), { stale: true });
      throw new Error('Tidak dapat terhubung ke server. Periksa koneksi lalu coba lagi.');
    }
    let data = null;
    try { data = await response.json(); } catch (_) { /* Empty responses are valid for writes. */ }
    if (currentGeneration !== generation) throw Object.assign(new Error('Respons lama diabaikan.'), { stale: true });
    if (!response.ok) {
      if (response.status === 401 && csrf) {
        signedOut();
        status('Sesi berakhir. Silakan masuk kembali.');
      }
      throw Object.assign(new Error(data && typeof data.error === 'string' ? data.error : `Permintaan gagal (HTTP ${response.status}). Silakan coba lagi.`), { status: response.status });
    }
    return data;
  }
  function array(data) {
    if (data === null) return [];
    if (!Array.isArray(data)) throw new Error('Format respons server tidak sesuai. Coba muat ulang data.');
    return data;
  }
  async function loadProjects() {
    $('project-count').textContent = '—';
    const projects = array(await api('/api/projects'));
    const previous = $('project-filter').value;
    $('project-filter').replaceChildren(new Option('Semua proyek', ''));
    projects.forEach((project) => $('project-filter').add(new Option(project, project)));
    if (projects.includes(previous)) $('project-filter').value = previous;
    $('project-count').textContent = String(projects.length);
  }
  async function loadNotes() {
    const id = ++requestID;
    const list = $('notes-list');
    list.setAttribute('aria-busy', 'true');
    empty(list, 'Memuat memori…');
    $('note-count').textContent = '—';
    $('skill-count').textContent = '—';
    status('Memuat memori…');
    const params = new URLSearchParams({ q: $('query').value.trim(), project: $('project-filter').value, kind: view === 'skills' ? 'skill' : $('kind-filter').value });
    try {
      const notes = array(await api(`/api/notes?${params}`));
      if (id !== requestID) return;
      list.replaceChildren();
      $('note-count').textContent = String(notes.length);
      $('skill-count').textContent = String(notes.filter((note) => note.kind === 'skill').length);
      notes.forEach((note) => {
        const card = text('article', '', 'note-card');
        const badges = text('div', '', 'badges');
        badges.append(text('span', note.project || 'Tanpa proyek', 'badge'), text('span', note.kind || 'memory', 'badge neutral'));
        if (note.archived) badges.append(text('span', 'Diarsipkan', 'badge neutral'));
        const heading = text('h3', '');
        const open = text('button', note.title || 'Tanpa judul', 'note-link');
        open.type = 'button';
        open.addEventListener('click', () => openNote(note.id));
        heading.append(open);
        card.append(badges, heading, text('p', (note.content || '').slice(0, 180), 'note-excerpt'), text('p', `${note.status || '—'} · Revisi ${note.revision} · ${date(note.updated_at)}`, 'card-meta'));
        list.append(card);
      });
      if (!notes.length) empty(list, 'Belum ada memori yang cocok. Coba kata kunci atau filter lain.');
      status(`${notes.length} memori ditemukan.`);
    } catch (error) {
      if (id !== requestID || error.stale) return;
      empty(list, 'Memori gagal dimuat. Gunakan tombol Muat ulang data untuk mencoba kembali.');
      status('Pemuatan memori gagal.');
      showError(error);
    } finally { if (id === requestID) list.setAttribute('aria-busy', 'false'); }
  }
  async function openNote(id) {
    const request = ++detailID;
    selectedNote = null;
    $('detail-title').textContent = 'Detail memori';
    $('detail-body').hidden = true;
    $('detail-error').hidden = true;
    $('detail-status').textContent = 'Memuat detail…';
    if (!$('detail-dialog').open) $('detail-dialog').showModal();
    try {
      const data = await api(`/api/notes/${encodeURIComponent(id)}`);
      if (request !== detailID) return;
      if (!data || !data.note || typeof data.html !== 'string') throw new Error('Detail memori tidak valid. Tutup dan buka kembali untuk mencoba lagi.');
      selectedNote = data.note;
      const note = data.note;
      $('detail-title').textContent = note.title || 'Tanpa judul';
      $('detail-meta').replaceChildren();
      Object.entries({ ID: note.id, Proyek: note.project, Jenis: note.kind, Status: note.archived ? 'Diarsipkan' : note.status, Sumber: note.source, Penulis: note.author, Revisi: note.revision, Dibuat: date(note.created_at), Diperbarui: date(note.updated_at) }).forEach(([label, value]) => {
        const pair = text('div', '');
        pair.append(text('dt', label), text('dd', value == null || value === '' ? '—' : value));
        $('detail-meta').append(pair);
      });
      // Only sanitized, server-rendered HTML from this same-origin API may use this sink.
      $('note-content').innerHTML = data.html;
      if (!data.html.trim()) empty($('note-content'), 'Memori ini belum memiliki isi.');
      $('publish-note').hidden = Boolean(note.archived) || note.kind !== 'skill' || note.status !== 'draft';
      $('archive-note').hidden = Boolean(note.archived);
      $('restore-note').hidden = !note.archived;
      $('note-history').replaceChildren();
      const history = array(data.history);
      history.forEach((revision) => {
        const item = text('article', '', 'history-item');
        item.append(text('h3', `Revisi ${revision.revision} · ${revision.title || 'Tanpa judul'}`), text('p', `${revision.author || '—'} · ${date(revision.updated_at)}`, 'muted'), text('pre', revision.content || '(Tanpa isi)', 'history-content'));
        $('note-history').append(item);
      });
      if (!history.length) empty($('note-history'), 'Belum ada riwayat revisi.');
      $('detail-body').hidden = false;
      $('detail-status').textContent = 'Detail memori dimuat.';
    } catch (error) {
      if (request !== detailID || error.stale) return;
      $('detail-status').textContent = 'Detail gagal dimuat.';
      $('detail-error').textContent = error.message;
      $('detail-error').hidden = false;
    }
  }
  async function changeNote(action) {
    if (!selectedNote) return;
    const note = selectedNote;
    const message = action === 'archive' ? `Arsipkan “${note.title}”? Memori akan ditandai sebagai arsip.` : action === 'restore' ? `Pulihkan “${note.title}” ke memori aktif?` : `Terbitkan “${note.title}” sebagai skill?`;
    if (!window.confirm(message)) return;
    $('publish-note').disabled = true;
    $('archive-note').disabled = true;
    $('restore-note').disabled = true;
    $('detail-error').hidden = true;
    $('detail-status').textContent = 'Menyimpan perubahan…';
    try {
      await api(`/api/notes/${encodeURIComponent(note.id)}/${action}`, { method: 'POST', body: { expected_revision: note.revision } });
      await openNote(note.id);
      await loadNotes();
    } catch (error) {
      if (error.stale || !csrf) return;
      $('detail-status').textContent = 'Perubahan belum tersimpan.';
      $('detail-error').textContent = error.status === 409 ? 'Revisi sudah berubah. Tutup dan buka kembali detail sebelum mencoba lagi.' : error.message;
      $('detail-error').hidden = false;
    } finally { $('publish-note').disabled = false; $('archive-note').disabled = false; $('restore-note').disabled = false; }
  }
  async function loadKeys() {
    const id = ++requestID;
    const list = $('keys-list');
    list.setAttribute('aria-busy', 'true');
    empty(list, 'Memuat kunci API…');
    status('Memuat kunci API…');
    try {
      const keys = array(await api('/api/keys'));
      if (id !== requestID) return;
      list.replaceChildren();
      keys.forEach((key) => {
        const item = text('article', '', 'key-card');
        const expired = key.expires_at && new Date(key.expires_at).getTime() < Date.now();
        const heading = text('div', '', 'section-heading');
        heading.append(text('h3', key.name || 'Tanpa nama'), text('span', key.revoked ? 'Dicabut' : expired ? 'Kedaluwarsa' : 'Aktif', 'badge'));
        item.append(heading, text('code', key.prefix || '—'), text('p', `Izin: ${(key.permissions || []).join(', ') || '—'} · Proyek: ${(key.projects || []).join(', ') || '—'}`), text('p', `Dibuat ${date(key.created_at)} · Kedaluwarsa: ${key.expires_at ? date(key.expires_at) : 'Tidak ditentukan'}`, 'muted'));
        if (!key.revoked) {
          const revoke = text('button', 'Cabut akses', 'danger');
          revoke.type = 'button';
          revoke.addEventListener('click', async () => {
            if (!window.confirm(`Cabut kunci “${key.name}”? Integrasi yang memakai kunci ini akan kehilangan akses. Tindakan ini tidak dapat dibatalkan.`)) return;
            revoke.disabled = true;
            clearError();
            try {
              await api(`/api/keys/${encodeURIComponent(key.id)}/revoke`, { method: 'POST', body: {} });
              status('Akses kunci berhasil dicabut.');
              if (view === 'keys') await loadKeys();
            } catch (error) { showError(error); revoke.disabled = false; }
          });
          item.append(revoke);
        }
        list.append(item);
      });
      if (!keys.length) empty(list, 'Belum ada kunci API. Buat kunci pertama untuk menghubungkan integrasi.');
      status(`${keys.length} kunci API dimuat.`);
    } catch (error) {
      if (id !== requestID || error.stale) return;
      empty(list, 'Kunci API gagal dimuat. Coba muat ulang data.');
      status('Pemuatan kunci API gagal.');
      showError(error);
    } finally { if (id === requestID) list.setAttribute('aria-busy', 'false'); }
  }
  async function loadActivity() {
    const id = ++requestID;
    const list = $('activity-list');
    list.setAttribute('aria-busy', 'true');
    empty(list, 'Memuat aktivitas…');
    status('Memuat aktivitas…');
    try {
      const entries = array(await api('/api/activity'));
      if (id !== requestID) return;
      list.replaceChildren();
      entries.forEach((entry) => {
        const item = text('article', '', 'activity-item');
        item.append(text('p', date(entry.time), 'activity-time'), text('h3', entry.action || 'Aktivitas'), text('p', `Pelaku: ${entry.actor || '—'} · Target: ${entry.target || '—'}`, 'muted'));
        list.append(item);
      });
      if (!entries.length) empty(list, 'Belum ada aktivitas yang tercatat.');
      status(`${entries.length} aktivitas dimuat.`);
    } catch (error) {
      if (id !== requestID || error.stale) return;
      empty(list, 'Aktivitas gagal dimuat. Coba muat ulang data.');
      status('Pemuatan aktivitas gagal.');
      showError(error);
    } finally { if (id === requestID) list.setAttribute('aria-busy', 'false'); }
  }
  function loadView() {
    clearError();
    return view === 'keys' ? loadKeys() : view === 'activity' ? loadActivity() : loadNotes();
  }
  function switchView(next, focus = true) {
    clearTimeout(searchTimer);
    requestID++;
    view = next;
    $('page-title').textContent = titles[view][0];
    $('page-description').textContent = titles[view][1];
    document.querySelectorAll('[data-view]').forEach((button) => {
      if (button.dataset.view === view) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
    $('notes-view').hidden = !['notes', 'skills'].includes(view);
    $('keys-view').hidden = view !== 'keys';
    $('activity-view').hidden = view !== 'activity';
    $('kind-filter').disabled = view === 'skills';
    if (view === 'skills') $('kind-filter').value = 'skill';
    else if (view === 'notes') $('kind-filter').value = '';
    $('list-heading').textContent = view === 'skills' ? 'Skill tersimpan' : 'Memori tersimpan';
    if (focus) $('main').focus();
    return loadView();
  }
  function signedIn(data) {
    if (!data || typeof data.csrf !== 'string' || !data.csrf) throw new Error('Sesi tidak valid: token CSRF tidak tersedia. Silakan masuk kembali.');
    csrf = data.csrf;
    $('password').value = '';
    $('login').hidden = true;
    $('dashboard').hidden = false;
    clearError();
    switchView('notes');
    loadProjects().catch(showError);
  }

  $('login-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    clearError();
    $('login-submit').disabled = true;
    status('Memverifikasi kata sandi…');
    try { signedIn(await api('/api/login', { method: 'POST', body: { password: $('password').value } })); }
    catch (error) { showError(error); status('Masuk belum berhasil.'); $('password').focus(); }
    finally { $('password').value = ''; $('login-submit').disabled = false; }
  });
  $('logout').addEventListener('click', async () => {
    $('logout').disabled = true;
    clearError();
    clearSecret();
    try { await api('/api/logout', { method: 'POST', body: {} }); signedOut(); status('Anda sudah keluar.'); }
    catch (error) { showError(error); status('Keluar belum terkonfirmasi oleh server. Coba lagi.'); }
    finally { $('logout').disabled = false; }
  });
  document.querySelectorAll('[data-view]').forEach((button) => button.addEventListener('click', () => switchView(button.dataset.view)));
  $('refresh').addEventListener('click', () => { loadView(); if (['notes', 'skills'].includes(view)) loadProjects().catch(showError); });
  $('filters').addEventListener('submit', (event) => { event.preventDefault(); clearTimeout(searchTimer); loadView(); });
  $('query').addEventListener('input', () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => loadView(), 300); });
  ['project-filter', 'kind-filter'].forEach((id) => $(id).addEventListener('change', () => { clearTimeout(searchTimer); loadView(); }));
  $('close-detail').addEventListener('click', () => $('detail-dialog').close());
  $('detail-dialog').addEventListener('close', () => { detailID++; selectedNote = null; $('note-content').replaceChildren(); });
  $('publish-note').addEventListener('click', () => changeNote('publish'));
  $('archive-note').addEventListener('click', () => changeNote('archive'));
  $('restore-note').addEventListener('click', () => changeNote('restore'));
  $('dismiss-token').addEventListener('click', () => { clearSecret(); $('key-name').focus(); status('Token ditutup dan tidak dapat ditampilkan kembali.'); });
  $('copy-token').addEventListener('click', async () => {
    const currentGeneration = generation;
    clearError();
    try {
      if (!navigator.clipboard) throw new Error('Clipboard tidak tersedia.');
      await navigator.clipboard.writeText($('new-token').value);
      if (currentGeneration === generation) status('Token disalin. Simpan di tempat aman.');
    } catch (_) {
      if (currentGeneration !== generation) return;
      $('new-token').focus();
      $('new-token').select();
      showError(new Error('Token tidak dapat disalin otomatis. Teks sudah dipilih; gunakan Ctrl+C atau Command+C, atau menu Salin perangkat.'));
    }
  });
  $('key-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    clearError();
    const name = $('key-name').value.trim();
    const permissions = Array.from(document.querySelectorAll('input[name="permission"]:checked'), (input) => input.value);
    const projects = [...new Set($('key-projects').value.split(',').map((part) => part.trim()))];
    let expiresAt = '';
    try {
      if (!name) { $('key-name').focus(); throw new Error('Isi nama kunci terlebih dahulu.'); }
      if (!permissions.length) { document.querySelector('input[name="permission"]').focus(); throw new Error('Pilih setidaknya satu izin akses.'); }
      if (!(projects.length === 1 && projects[0] === '*') && !projects.every((project) => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(project))) {
        $('key-projects').focus(); throw new Error('Isi slug proyek yang valid, dipisahkan koma; atau gunakan * saja untuk semua proyek.');
      }
      if ($('key-expiry').value) {
        const expires = new Date(`${$('key-expiry').value}T23:59:59`);
        if (Number.isNaN(expires.getTime()) || expires.getTime() <= Date.now()) { $('key-expiry').focus(); throw new Error('Tanggal kedaluwarsa harus di masa depan.'); }
        expiresAt = expires.toISOString();
      }
      if (!$('secret-panel').hidden && !window.confirm('Token sebelumnya akan ditutup. Sudah menyimpannya dengan aman?')) return;
      $('create-key').disabled = true;
      status('Membuat kunci API…');
      const data = await api('/api/keys', { method: 'POST', body: { name, permissions, projects, expires_at: expiresAt } });
      if (!data || typeof data.token !== 'string' || !data.token) throw new Error('Server tidak mengembalikan token. Periksa daftar kunci sebelum mencoba lagi.');
      clearSecret();
      $('new-token').value = data.token;
      $('secret-panel').hidden = false;
      $('key-form').reset();
      if (view === 'keys') {
        $('new-token').focus();
        await loadKeys();
      }
      status('Kunci dibuat. Token tersedia sekali di halaman Kunci API; simpan sekarang.');
    } catch (error) { showError(error); }
    finally { $('create-key').disabled = false; }
  });

  api('/api/session').then(signedIn).catch((error) => {
    signedOut();
    status('Silakan masuk untuk melanjutkan.');
    if (error.status !== 401) showError(error);
  });
})();
