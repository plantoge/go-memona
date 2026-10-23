import test from 'node:test';
import assert from 'node:assert/strict';
import type { Note } from '../src/lib/types.ts';
import { compareRevisionMetadata } from '../src/lib/revision-metadata.ts';

const note: Note = { id:'a'.repeat(32),project:'demo',kind:'skill',title:'Synthetic',content:'procedure',source:'evidence',status:'draft',author:'fixture',revision:1,created_at:'',updated_at:'',archived:false };
test('metadata comparison detects declarations and lifecycle, not just content', () => {
  const changed = { ...note, title:'<script>text only</script>', status:'active', is_testing:true, skill_metadata: {trigger:'Review fixtures',version:'1.0',compatibility:['OpenCode'],test_evidence:'test report'} };
  const rows = compareRevisionMetadata(note, changed);
  assert.deepEqual(rows.map(r => r.label), ['Judul','Status','Testing','Pemicu skill','Versi skill','Kompatibilitas skill','Bukti pengujian skill']);
  assert.equal(rows[0].after, '<script>text only</script>');
  assert.equal(rows.every(r => !r.truncated), true);
});
test('omitted flags and reordered relations are semantically unchanged', () => {
  const before = { ...note, related_to:['b','a'] };
  assert.deepEqual(compareRevisionMetadata(before, { ...before, is_testing:false, is_project_summary:false, related_to:['a','b'], revision:2, content:'new body',updated_at:'later' }), []);
  assert.deepEqual(before.related_to, ['b','a']);
});
test('long values are bounded and marked, but equal prefixes do not hide change', () => {
  const rows = compareRevisionMetadata({...note,source:'x'.repeat(2001)}, {...note,source:'x'.repeat(2000)+'y'});
  assert.equal(rows.length, 1);
  assert.equal(rows[0].truncated, true);
  assert.equal(rows[0].before.length, 2000);
  assert.equal(rows[0].after.length, 2000);
});
