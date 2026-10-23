<script lang="ts">
  import { date, type StorageHealth } from '../types';
  import { bytes, issueLabel } from '../storage-health';
  import Icon from './Icon.svelte';
  let { report, loading, onrefresh }: { report: StorageHealth | null; loading: boolean; onrefresh: () => void } = $props();
</script>

<section aria-labelledby="storage-heading" aria-busy={loading} class="stack">
  <div class="section-heading"><h2 id="storage-heading">Kesehatan penyimpanan</h2><button class="secondary" type="button" disabled={loading} onclick={onrefresh}><Icon name="refresh" />Periksa ulang</button></div>
  <p class="muted">Pemeriksaan read-only. Tidak ada file yang diperbaiki, dihapus, atau ditulis untuk uji coba. Khusus administrator; bukan pemeriksaan SQLite atau jaminan backup.</p>
  {#if loading}
    <p class="empty-state" role="status">Memeriksa penyimpanan…</p>
  {:else if report}
    <article class="panel">
      <h3>{report.status === 'ok' ? 'Tidak ada masalah yang terdeteksi' : report.status === 'warning' ? 'Perlu perhatian' : 'Ditemukan masalah atau pemeriksaan tidak lengkap'}</h3>
      <p class="muted">Diperiksa: {date(report.checked_at)} · {report.integrity.complete ? 'Pemindaian selesai' : 'Pemindaian tidak lengkap — bukan hasil bersih'}</p>
      <dl class="key-details">
        <dt>Disk tersedia</dt><dd>{report.disk.available ? `${bytes(report.disk.available_bytes)} dari ${bytes(report.disk.total_bytes)}` : 'Tidak tersedia'}</dd>
        <dt>Inode tersedia</dt><dd>{report.disk.available ? report.disk.available_inodes.toLocaleString('id-ID') : 'Tidak tersedia'}</dd>
        <dt>Direktori catatan</dt><dd>{report.integrity.notes} diperiksa</dd>
        <dt>Revisi canonical</dt><dd>{report.integrity.revisions} terbaca; lihat juga temuan urutan/transisi histori</dd>
        <dt>Data diperiksa</dt><dd>{bytes(report.integrity.bytes)} · {report.integrity.entries} entri</dd>
        <dt>Staging tertinggal</dt><dd>{report.staging_entries} entri tingkat atas; tidak dibuka atau dibersihkan</dd>
        <dt>Kegagalan tulis</dt><dd>{report.write_failures_observed ? `${report.write_failures.count} sejak proses mulai` : 'Tidak diamati pada pemeriksaan offline'}{#if report.write_failures.last_at}<br>{date(report.write_failures.last_at)} · {report.write_failures.last_code}{/if}</dd>
      </dl>
      {#if report.disk.low}<p class="inline-error">Ruang atau inode disk rendah. Hentikan penambahan beban dan rencanakan pemulihan kapasitas; jangan menghapus revisi atau staging tanpa review.</p>{/if}
      {#if report.staging_entries > 0}<p class="hint">Staging dapat berasal dari write terputus. Jumlah ini bukan bukti kehilangan data. Pertahankan file dan periksa salinan sebelum tindakan pemulihan.</p>{/if}
    </article>
    {#if Object.keys(report.integrity.issues).length > 0}
      <article class="panel"><h3>Temuan integritas</h3><ul>{#each Object.entries(report.integrity.issues) as [code, count]}<li>{issueLabel(code)}: {count}</li>{/each}</ul><p class="muted">Hitungan temuan dapat tumpang tindih, bukan jumlah catatan rusak. Pertahankan semua file, hentikan writer secara normal dengan persetujuan, dan periksa salinan yang konsisten. Jangan mengedit histori.</p></article>
    {/if}
    <p class="hint">Maksimum 20.000 entri / 256 MiB / 5 detik. Kegagalan tulis hanya I/O atau korupsi pada mutasi Markdown sejak proses mulai; nol bukan bukti disk dapat ditulis. Ruang rendah: kurang dari 256 MiB, 5% kapasitas, atau 100 inode. Hasil adalah snapshot, bukan pemantauan berkelanjutan.</p>
  {:else}
    <p class="empty-state">Laporan belum tersedia. Gunakan Periksa ulang; hasil lama tidak ditampilkan sebagai hasil terkini.</p>
  {/if}
</section>

<style>
  .section-heading { flex-wrap: wrap; }
  .key-details { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  @media (max-width: 760px) { .key-details { grid-template-columns: 1fr; gap: 6px; } .key-details dd { margin-bottom: 12px; } }
</style>
