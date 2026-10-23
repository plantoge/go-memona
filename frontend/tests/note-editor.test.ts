import test from 'node:test';
import assert from 'node:assert/strict';
import { initialNoteInput, noteEditorErrors, noteEditorInput, compareNoteLines, canonicalVerificationTime, verificationTimeNow } from '../src/lib/note-editor.ts';

const valid = { project: 'demo', kind: 'knowledge', title: 'Judul', content: '# Isi\nbaris kedua', source: '', status: 'unverified' };
test('editor sends complete replacement content without metadata or response fields', () => {
  assert.deepEqual(noteEditorErrors(valid), {});
  assert.deepEqual(noteEditorInput({ ...valid, related_to: ['hidden'], revision: 7 } as typeof valid), valid);
  assert.deepEqual(initialNoteInput(null, 'demo'), { ...valid, title: '', content: '' });
});
test('editor validates byte bounds, slug, controls, required title and source evidence', () => {
  for (const change of [
    { project: '../bad' }, { kind: 'unknown' }, { title: '  ' }, { title: 'é'.repeat(101) },
    { title: 'bad\nvalue' }, { source: 'é'.repeat(1001) }, { source: 'bad\tvalue' },
    { content: 'é'.repeat(32769) }, { content: '\0' }, { status: 'active' },
    { status: 'verified', source: '' }, { kind: 'skill', status: 'verified', source: 'Evidence' },
  ]) assert.ok(Object.keys(noteEditorErrors({ ...valid, ...change })).length);
  assert.deepEqual(noteEditorErrors({ ...valid, status: 'verified', source: 'Evidence' }), {});
  assert.deepEqual(noteEditorErrors({ ...valid, kind: 'skill', status: 'draft' }), {});
  assert.throws(() => noteEditorInput({ ...valid, title: '' }));
});
test('active skills start at draft and metadata remains omitted', () => {
  const note = { ...valid, id: 'a'.repeat(32), kind: 'skill', status: 'active', revision: 2, author: 'admin', archived: false, created_at: '', updated_at: '', is_project_summary: true, related_to: ['b'.repeat(32)] };
  const input = initialNoteInput(note);
  assert.equal(input.status, 'draft');
  assert.equal(input.project, note.project);
  assert.equal('related_to' in input, false);
  assert.equal('is_project_summary' in input, false);
});
test('skill review metadata is bounded and copied without aliases', () => {
  const metadata = { trigger: 'Review kode', version: '1.0', compatibility: ['OpenCode'], test_evidence: 'synthetic test' };
  const input = { ...valid, kind: 'skill', status: 'draft', skill_metadata: metadata };
  assert.deepEqual(noteEditorErrors(input), {});
  const sent = noteEditorInput(input);
  sent.skill_metadata!.compatibility.push('Hermes');
  assert.deepEqual(metadata.compatibility, ['OpenCode']);
  const minimal = initialNoteInput({ ...valid, kind:'skill', id:'a'.repeat(32), revision:1, author:'test', archived:false, created_at:'', updated_at:'', skill_metadata: { version:'1.0' } as typeof metadata });
  assert.deepEqual(minimal.skill_metadata?.compatibility, []);
  assert.equal(minimal.skill_metadata?.trigger, '');
  assert.ok(noteEditorErrors({ ...input, kind: 'fact', status: 'unverified' }).skill_metadata);
  assert.ok(noteEditorErrors({ ...input, skill_metadata: { ...metadata, trigger: 'é'.repeat(501) } }).skill_metadata);
});
test('review fields preserve declared time, clear explicitly and validate status/UTC', () => {
  assert.equal('verified_at' in noteEditorInput(valid), false);
  const input = { ...valid, status:'verified', source:'checked against fixture', verified_at:'2026-10-23T09:00:00Z', needs_review:true };
  assert.deepEqual(noteEditorErrors(input), {});
  assert.equal(noteEditorInput(input).needs_review, true);
  assert.equal(noteEditorInput({...valid,verified_at:'',needs_review:false}).verified_at, '');
  assert.ok(noteEditorErrors({...input,status:'unverified'}).verified_at);
  assert.ok(noteEditorErrors({...input,verified_at:'2026-10-23T09:00:00+07:00'}).verified_at);
  assert.ok(noteEditorErrors({...input,verified_at:'not a timestamp'}).verified_at);
  assert.equal(canonicalVerificationTime('2026-02-30T09:00:00Z'), false);
  assert.equal(canonicalVerificationTime('2026-10-23T09:00:00.100Z'), false);
  assert.equal(canonicalVerificationTime('2026-10-23T09:00:00.123456789Z'), true);
  assert.equal(canonicalVerificationTime(verificationTimeNow()), true);
  assert.ok(noteEditorErrors({...valid,kind:'skill',status:'draft',needs_review:false}).verified_at);
});
test('testing flag is explicit, omission stays omitted and false clears', () => {
  assert.equal('is_testing' in noteEditorInput(valid), false);
  assert.equal(noteEditorInput({ ...valid, is_testing: true }).is_testing, true);
  assert.equal(noteEditorInput({ ...valid, is_testing: false }).is_testing, false);
});
test('bounded positional diff treats markup as plain text and marks additions/removals', () => {
  const result = compareNoteLines('<script>alert(1)</script>\nsame\nremoved', '<img src=x onerror=alert(1)>\nsame');
  assert.equal(result.lines[0].before, '<script>alert(1)</script>');
  assert.equal(result.lines[0].changed, true);
  assert.equal(result.lines[1].changed, false);
  assert.equal(result.lines[2].after, null);
  assert.equal(result.truncated, false);
  assert.equal(compareNoteLines('', 'one\ntwo').lines[1].before, null);
  const lines = compareNoteLines('x\n'.repeat(10000), 'x'.repeat(50000));
  assert.equal(lines.lines.length, 200);
  assert.equal(lines.truncated, true);
  assert.equal(compareNoteLines('x'.repeat(20001), '').truncated, true);
});
