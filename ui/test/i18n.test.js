// The page's words: both languages say everything, with the same
// placeholders, in their own letters, with a plain hyphen wherever a person
// reads a dash - and every key the page asks for is there.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

import { SAME_IN_BOTH, STRINGS, pickLanguage, translator } from '../lib/i18n.js';

const here = dirname(fileURLToPath(import.meta.url));

const placeholders = (s) => [...s.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort();

test('both languages have the same keys, none of them empty', () => {
  const en = Object.keys(STRINGS.en).sort();
  const tr = Object.keys(STRINGS.tr).sort();
  assert.deepEqual(tr, en);
  for (const [lang, table] of Object.entries(STRINGS)) {
    for (const [k, v] of Object.entries(table)) {
      assert.ok(typeof v === 'string' && v.trim() !== '', `${lang} ${k} is empty`);
    }
  }
});

test('a sentence has the same placeholders in both languages', () => {
  for (const k of Object.keys(STRINGS.en)) {
    assert.deepEqual(placeholders(STRINGS.tr[k]), placeholders(STRINGS.en[k]), k);
  }
});

test('Turkish is translated, not copied', () => {
  for (const k of Object.keys(STRINGS.en)) {
    if (SAME_IN_BOTH.has(k)) continue;
    assert.notEqual(STRINGS.tr[k], STRINGS.en[k], `${k} reads the same in Turkish`);
  }
});

test('Turkish keeps its own letters', () => {
  const all = Object.values(STRINGS.tr).join('\n');
  for (const letter of ['ı', 'İ', 'ş', 'ğ', 'ü', 'ö', 'ç']) {
    assert.ok(all.includes(letter), `no "${letter}" anywhere in the Turkish strings`);
  }
  // Words a keyboard without Turkish letters writes: none of them may appear.
  const ascii = /\b(Yukleniyor|onizleme|Onizleme|gosterilemiyor|goruntu|Goruntu|dosyasi|Agirlik|Agirliklar|Ozellik|Olcu|Bicim|Surum|Tasarimci|Ureticisi|Uretici|Kucuk|kucuk|degil|Degil|icin|Icin|sifreli|Italik|Genislik|Cok|Yari|Kalin|Ince)\b/;
  for (const [k, v] of Object.entries(STRINGS.tr)) {
    assert.doesNotMatch(v, ascii, `${k}: "${v}" is written without its Turkish letters`);
  }
});

test('a dash a person reads is a plain hyphen', () => {
  for (const [lang, table] of Object.entries(STRINGS)) {
    for (const [k, v] of Object.entries(table)) {
      assert.doesNotMatch(v, /[\u2012\u2013\u2014\u2015\u2212]/, `${lang} ${k} has a long dash`);
    }
  }
});

test('the page speaks Turkish to a Turkish filex and English to everyone else', () => {
  assert.equal(pickLanguage('tr'), 'tr');
  assert.equal(pickLanguage('tr-TR'), 'tr');
  assert.equal(pickLanguage('TR'), 'tr');
  assert.equal(pickLanguage('en-GB'), 'en');
  assert.equal(pickLanguage('es'), 'en');
  assert.equal(pickLanguage(''), 'en');
  assert.equal(pickLanguage(undefined), 'en');

  const t = translator('tr');
  assert.equal(t('font.size_px', { size: 12 }), '12 px');
  assert.equal(t('fact.size_value', { width: 10, height: 20 }), '10 × 20 piksel');
  assert.equal(t('no.such.key'), 'no.such.key', 'a missing key shows itself');
  assert.equal(translator('en')('problem.no_preview', { program: 'Affinity' }).includes('Affinity'), true);
});

// Every key the page asks for by name, and every key it builds from a value
// the module answers (internal/design psdModeNames, the formats, the
// problems of internal/app), is in the catalogue.
test('every key the page uses is in the catalogue', () => {
  const sources = ['../main.js', '../lib/view.js'].map((f) => readFileSync(join(here, f), 'utf8'));
  const named = new Set();
  for (const src of sources) {
    for (const m of src.matchAll(/\bt\('([a-z0-9_.]+)'/g)) named.add(m[1]);
  }
  assert.ok(named.size > 30, `only ${named.size} keys found: the scan is broken`);
  const built = [
    ...['bitmap', 'grayscale', 'indexed', 'rgb', 'cmyk', 'multichannel', 'duotone', 'lab'].map((m) => `mode.${m}`),
    ...['psd', 'psb', 'affinity', 'pixelmator'].map((f) => `format.${f}`),
    ...['truetype', 'opentype', 'collection', 'woff', 'woff2'].map((f) => `font.format.${f}`),
    ...[100, 200, 300, 400, 500, 600, 700, 800, 900].map((w) => `weight.${w}`),
    ...['real', 'not_real'].map((m) => `fact.merged.${m}`),
    ...['encrypted', 'too_large', 'timeout', 'cython', 'not_a_font', 'damaged', 'unavailable'].map((p) => `problem.${p}`),
  ];
  for (const k of [...named, ...built]) {
    assert.ok(k in STRINGS.en, `the page asks for "${k}", which the catalogue does not have`);
  }
});
