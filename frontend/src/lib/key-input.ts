export function keyInput(name: string, permissions: string[], rawProjects: string, expiry: string, now = Date.now()) {
  name = name.trim();
  if (!name) throw new Error('Isi nama kunci terlebih dahulu.');
  if (!permissions.length) throw new Error('Pilih setidaknya satu izin akses.');
  const projects = [...new Set(rawProjects.split(',').map(part => part.trim()))];
  if (!(projects.length === 1 && projects[0] === '*') && !projects.every(project => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(project))) {
    throw new Error('Isi slug proyek yang valid, dipisahkan koma; atau gunakan * saja untuk semua proyek.');
  }
  let expires_at = '';
  if (expiry) {
    const expires = new Date(`${expiry}T23:59:59`);
    if (Number.isNaN(expires.getTime()) || expires.getTime() <= now) throw new Error('Tanggal kedaluwarsa harus di masa depan.');
    expires_at = expires.toISOString();
  }
  return { name, permissions, projects, expires_at };
}
