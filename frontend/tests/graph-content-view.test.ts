import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { compile } from 'svelte/compiler';

// Compile/contract regressions, NOT browser/DOM/visual interaction coverage.
const source = (name:string) => readFileSync(new URL(`../src/${name}`,import.meta.url),'utf8');
test('graph wiring opens existing authenticated detail and suspends its background renderer', () => {
  const app = source('App.svelte');
  assert.match(app,/suspended=\{detailOpen\}/);
  assert.match(app,/contentFirst=\{view === 'graph'\}/);
  assert.match(app,/class:graph-workspace=\{view === 'graph'\}/);
});
test('graph-first detail has exclusive content-first and standard safe-rendered branches', () => {
  const detail = source('lib/components/NoteDetail.svelte');
  assert.match(detail, /\{#if contentFirst\}<article/);
  assert.match(detail, /\{#if !contentFirst\}<article/);
  assert.ok(detail.indexOf('{#if contentFirst}') < detail.indexOf('<dl class="metadata">'));
  assert.doesNotMatch(detail, /\{@html\s+(?:note\.content|detail\.note\.content)/);
  const compiled = compile(detail,{filename:'NoteDetail.svelte',generate:'client'});
  assert.equal(compiled.warnings.length,0);
});
test('graph-first surface has compact controls and opens note content without an explorer', () => {
  const graph = source('lib/components/KnowledgeGraph.svelte');
  assert.match(graph,/onselect=\{open\}/);
  assert.match(graph,/paused=\{paused \|\| suspended \|\| loading\}/);
  for (const label of ['Perkecil','Perbesar','Reset tampilan graph']) assert.ok(graph.includes(`aria-label="${label}"`));
  assert.match(graph,/aria-pressed=\{paused\}/);
  assert.match(graph,/name=\{paused \? 'play' : 'pause'\}/);
  assert.doesNotMatch(graph,/Penjelajah daftar|Kontrol kamera tanpa gesture|class="inspector"/);
  assert.match(graph,/onkeydown=\{key\}/);
  const compiled = compile(graph,{filename:'KnowledgeGraph.svelte',generate:'client'});
  assert.equal(compiled.warnings.length,0);
});
test('canvas fallback remains operable without restoring a permanent explorer', () => {
  const canvas = source('lib/components/ThreeGraphCanvas.svelte');
  assert.match(canvas,/\{#if failed\}/);
  assert.match(canvas,/<select[^>]*onchange=/);
  assert.match(canvas,/onopen\(event.currentTarget.value\)/);
  assert.doesNotMatch(canvas,/Penjelajah daftar/);
  const compiled = compile(canvas,{filename:'ThreeGraphCanvas.svelte',generate:'client'});
  assert.equal(compiled.warnings.length,0);
});
