export function bytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return 'Tidak tersedia';
  if (value === 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** unit).toLocaleString('id-ID', { maximumFractionDigits: 1 })} ${units[unit]}`;
}
export function issueLabel(code: string): string {
  return ({
    disk_unavailable: 'Kapasitas disk tidak tersedia', directory_permissions: 'Izin atau pemilik direktori tidak sesuai',
    directory_unreadable: 'Direktori tidak terbaca', note_directory_unreadable: 'Direktori catatan tidak terbaca',
    unexpected_note_entry: 'Entri catatan tidak dikenal', empty_note_directory: 'Direktori catatan kosong',
    revision_sequence: 'Nama atau urutan revisi tidak valid', unsafe_revision: 'Tipe, ukuran, izin atau pemilik revisi tidak aman',
    corrupt_revision: 'Revisi rusak atau tidak canonical', history_transition: 'Transisi histori tidak valid',
    duplicate_operation: 'Bukti operasi ganda', duplicate_summary: 'Ringkasan proyek ganda',
    invalid_relation: 'Target relasi tidak valid atau tidak dapat diperiksa', supersedes_cycle: 'Siklus supersedes',
    entry_limit: 'Batas 20.000 entri tercapai', byte_limit: 'Batas pembacaan 256 MiB tercapai',
    cancelled_or_timeout: 'Pemeriksaan dibatalkan atau melewati 5 detik',
  } as Record<string, string>)[code] || code;
}
