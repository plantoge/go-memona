<script lang="ts">
  import { integrationSnippet, type Harness } from '../integration-guide';
  let { endpoint, oncreate, busy }: { endpoint: string; oncreate: (name: string) => void; busy: boolean } = $props();
  let harness = $state<Harness>('opencode'), version = $state(''), profile = $state(''), feedback = $state('');
  let configField: HTMLTextAreaElement;
  const snippet = $derived.by(() => { try { return integrationSnippet(harness, endpoint); } catch { return ''; } });
  async function copy() {
    const previous = snippet; feedback = '';
    try { await navigator.clipboard.writeText(previous); if (snippet === previous) feedback = 'Snippet disalin tanpa secret.'; }
    catch { if (snippet === previous) { configField?.focus(); configField?.select(); feedback = 'Salin otomatis tidak tersedia. Gunakan Ctrl+C / Command+C pada teks terpilih.'; } }
  }
</script>
<section class="panel" aria-labelledby="integration-heading">
  <h2 id="integration-heading">Panduan integrasi</h2>
  <p class="hint">Konfigurasi dibantu langkah demi langkah; tidak memasang atau mengubah harness otomatis. Hasil uji dicatat oleh Anda, bukan status koneksi langsung.</p>
  <div class="filters"><label>Harness<select bind:value={harness} onchange={() => { feedback = ''; }}><option value="opencode">OpenCode V2</option><option value="hermes">Hermes</option></select></label><label>Versi terpasang (catatan lokal)<input bind:value={version} maxlength="80" placeholder="Cocokkan dengan dokumentasi"></label><label>Profil / workspace (catatan lokal)<input bind:value={profile} maxlength="120" placeholder="Profil yang akan dihubungkan"></label></div>
  <ol class="stack">
    <li><h3>Buat key terpisah</h3><p>Pilih proyek, izin minimum dan masa berlaku pada formulir key. Scope * berarti semua proyek, termasuk proyek baru. Token hanya muncul sekali.</p><button type="button" class="secondary" disabled={busy} onclick={() => oncreate(`${harness === 'opencode' ? 'OpenCode' : 'Hermes'}${profile.trim() ? ` ${profile.trim()}` : ''}`.slice(0, 100))}>Pilih scope dan buat key</button></li>
    <li><h3>Gabungkan konfigurasi di mesin klien</h3><p>{harness === 'opencode' ? 'Gabungkan entry mcp.servers.memona ke opencode.json atau opencode.jsonc proyek yang benar. Gunakan /mcps untuk reconnect dan opencode mcp list untuk memeriksa koneksi.' : 'Gabungkan entry mcp_servers.memona ke config.yaml profil Hermes aktif. Muat ulang melalui /reload-mcp. Memori bawaan Hermes tetap dipertahankan.'} Jangan menimpa entry lain; pilih nama server lain bila memona sudah dipakai.</p>
      <label>Snippet {harness === 'opencode' ? 'JSON' : 'YAML'} tanpa token<textarea bind:this={configField!} value={snippet} readonly rows="12" spellcheck="false"></textarea></label><button type="button" class="secondary" disabled={!snippet} onclick={copy}>Salin snippet</button>{#if feedback}<p role="status" class="hint">{feedback}</p>{/if}
      <p class="hint">Simpan token dalam MEMONA_API_KEY di environment proses yang membuat koneksi MCP. Shell baru tidak mengubah environment service lama. Loopback menunjuk mesin harness, bukan server remote. Jangan menaruh token di URL, Git, chat atau screenshot.</p>
    </li>
    <li><h3>Verifikasi discovery dan izin</h3><p>Pastikan sepuluh tool ditemukan, baca get_memory_guide, lalu get_context pada proyek yang diizinkan. Jika write dipilih, simpan catatan sintetis dengan operation_id unik dan baca kembali jika read tersedia. Uji penolakan proyek di luar scope; key read-only harus gagal menulis. Status connected saja bukan bukti acceptance.</p><p class="hint">Versi: {version || 'belum dicatat'} · Profil: {profile || 'belum dicatat'}. Catatan ini hanya di memori halaman dan hilang saat keluar.</p></li>
  </ol>
  <p class="hint">Bentuk snippet diperiksa terhadap dokumentasi resmi; kompatibilitas versi terpasang tetap perlu diuji.</p>
  <a href={harness === 'opencode' ? 'https://opencode.ai/v2/docs/mcp-servers' : 'https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp/'} target="_blank" rel="noopener noreferrer">Dokumentasi resmi {harness === 'opencode' ? 'OpenCode V2' : 'Hermes'} (tab baru)</a>
</section>
<style>
  textarea { width: 100%; box-sizing: border-box; resize: vertical; font-family: monospace; } label { display: grid; gap: .4rem; }
  li { padding-inline-start: .3rem; } ol { padding-inline-start: 1.5rem; } p { overflow-wrap: anywhere; }
</style>
