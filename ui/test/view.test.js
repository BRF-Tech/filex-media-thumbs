// What the page shows, from the module's answers (lib/view.js).
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { translator } from '../lib/i18n.js';
import {
  CHARS_FIRST,
  base64ToBytes,
  captionOf,
  codepointLabel,
  defaultSample,
  designFacts,
  fontDraws,
  fontFacts,
  fontTitle,
  printableCodepoints,
  problemText,
  programOf,
  shownType,
  variationSettings,
  weightName,
  weightRows,
} from '../lib/view.js';

const en = translator('en');
const tr = translator('tr');

test('a static font is shown at the one weight it is, never at a faked one', () => {
  assert.deepEqual(weightRows({ weight: 700, axes: [] }), [{ label: '', coords: null, weight: 700 }]);
  assert.deepEqual(weightRows({}), [{ label: '', coords: null, weight: 400 }], 'no OS/2: regular');
});

test('a variable font is shown along its weight axis, its ends included', () => {
  const rows = weightRows({ axes: [{ tag: 'wght', min: 250, default: 400, max: 820 }] });
  assert.deepEqual(
    rows.map((r) => r.weight),
    [250, 300, 400, 500, 600, 700, 800, 820],
  );
  assert.deepEqual(rows[0].coords, { wght: 250 });
});

test('a variable font with named instances is shown by them, every axis set', () => {
  const rows = weightRows({
    axes: [{ tag: 'wght', min: 100, default: 400, max: 900 }, { tag: 'wdth', min: 75, default: 100, max: 100 }],
    instances: [
      { name: 'Thin Condensed', coords: { wght: 100, wdth: 75 } },
      { name: 'Bold', coords: { wght: 700, wdth: 100 } },
    ],
  });
  assert.deepEqual(
    rows.map((r) => r.label),
    ['Thin Condensed', 'Bold'],
  );
  assert.equal(variationSettings(rows[0].coords), '"wght" 100, "wdth" 75');
});

test('variation settings carry only tags and numbers', () => {
  assert.equal(variationSettings(null), '');
  assert.equal(variationSettings({ 'a"b\\': 1, wght: 400, slnt: Number.NaN, 'toolong': 3 }), '"wght" 400');
});

test('weights are named in the reader language', () => {
  assert.equal(weightName(en, 700), 'Bold');
  assert.equal(weightName(tr, 700), 'Kalın');
  assert.equal(weightName(tr, 360), 'Normal', 'the nearest hundred');
  assert.equal(weightName(en, 1000), 'Black', 'clamped');
  assert.equal(weightName(en, 50), 'Thin', 'clamped');
  assert.equal(weightName(en, 0), 'Regular', 'no weight class: regular');
});

test('the character grid leaves out controls, spaces and surrogates', () => {
  assert.deepEqual(printableCodepoints([0x09, 0x20, 0x41, 0x7f, 0xa0, 0xad, 0xe9, 0xd800, 0x1f600, 0x110000]), [0x41, 0xe9, 0x1f600]);
  assert.deepEqual(printableCodepoints(null), []);
  assert.equal(codepointLabel(0x41), 'U+0041');
  assert.equal(codepointLabel(0x1f600), 'U+1F600');
  assert.equal(CHARS_FIRST, 256);
});

test('the sample is the reader sentence when the font draws it, else its own letters', () => {
  const latin = [...'The quick brown fox jumps over the lazy dog'].map((c) => c.codePointAt(0));
  assert.equal(defaultSample(en, latin), 'The quick brown fox jumps over the lazy dog');
  assert.ok(fontDraws(latin, 'fox dog'));
  assert.ok(!fontDraws(latin, 'şoföre'), 'a Latin font without Turkish letters');
  assert.ok(fontDraws(null, 'anything'), 'an unread map says yes');

  const greek = [0x391, 0x392, 0x393];
  assert.equal(defaultSample(tr, greek), 'ΑΒΓ', 'a Greek font shows its own letters');
  const turkish = [...'Pijamalı hasta yağız şoföre çabucak güvendi'].map((c) => c.codePointAt(0));
  assert.equal(defaultSample(tr, turkish), 'Pijamalı hasta yağız şoföre çabucak güvendi');
});

test('a Photoshop file says what it is, in the reader language', () => {
  const facts = { format: 'psd', width: 1200, height: 800, mode: 'cmyk', depth: 16, channels: 5, layers: 0, merged: 'not_real' };
  const rows = designFacts(tr, 'tr', facts);
  const byKey = Object.fromEntries(rows.map((r) => [r.key, r]));
  assert.equal(byKey.format.value, 'Photoshop belgesi (PSD)');
  assert.equal(byKey.size.value, '1.200 × 800 piksel');
  assert.equal(byKey.mode.value, 'CMYK renk');
  assert.equal(byKey.depth.value, 'Kanal başına 16 bit');
  assert.equal(byKey.layers.value, '0', 'no layers is said, not left out');
  assert.match(byKey.merged.value, /Kaydedilmemiş/);
  assert.deepEqual(
    rows.map((r) => r.key),
    ['format', 'size', 'mode', 'depth', 'channels', 'layers', 'merged'],
  );
});

test('an Affinity and a Pixelmator Pro file say what they are', () => {
  const af = designFacts(en, 'en', { format: 'affinity', container: 12, generation: '3', document: 'afphoto' });
  assert.deepEqual(
    af.map((r) => [r.key, r.value]),
    [
      ['format', 'Affinity document'],
      ['document', 'Affinity Photo'],
      ['container', '12 (Affinity 3)'],
    ],
  );
  const pxd = designFacts(en, 'en', { format: 'pixelmator', members: 4, preview_member: 'QuickLook/Thumbnail.webp' });
  assert.deepEqual(
    pxd.map((r) => r.key),
    ['format', 'members', 'preview_member'],
  );
});

test('every problem has its sentence', () => {
  assert.match(problemText(en, 'no_preview', 'Affinity', 'affinity'), /This Affinity file holds no preview/);
  assert.match(problemText(en, 'no_preview', 'Adobe Photoshop', 'psd'), /Maximize Compatibility/);
  assert.match(problemText(tr, 'cython', 'Pixelmator Pro', 'pixelmator'), /Cython bildirim dosyası/);
  assert.match(problemText(en, 'not_this_format', 'Pixelmator Pro', 'pixelmator'), /not a Pixelmator Pro file/);
  for (const p of ['encrypted', 'too_large', 'timeout', 'not_a_font', 'damaged', 'unavailable']) {
    assert.notEqual(problemText(en, p, 'X', 'psd'), `problem.${p}`, p);
    assert.notEqual(problemText(tr, p, 'X', 'psd'), problemText(en, p, 'X', 'psd'), `${p} in Turkish`);
  }
  assert.equal(problemText(en, 'something new', 'X', 'psd'), en('problem.damaged'), 'an unknown problem reads as damage');
});

test('the caption says where the picture came from', () => {
  assert.match(captionOf(en, { source: 'merged', width: 320, height: 200 }, 'Adobe Photoshop'), /merged image.*320 × 200/);
  assert.match(captionOf(en, { source: 'thumbnail', width: 160, height: 100 }, 'Adobe Photoshop'), /thumbnail Adobe Photoshop saved/);
  assert.match(captionOf(tr, { source: 'QuickLook/Thumbnail.webp', width: 1024, height: 1024 }, 'Pixelmator Pro'), /Pixelmator Pro programının/);
  assert.equal(captionOf(en, null, 'X'), '');
});

test('a font says what it is', () => {
  const info = {
    format: 'woff2',
    outlines: 'truetype',
    family: 'Go',
    style: 'Regular',
    full_name: 'Go Regular',
    weight: 400,
    glyphs: 652,
    axes: [{ tag: 'wght', min: 100, default: 400, max: 900 }],
    version: 'Version 2.010',
  };
  const rows = fontFacts(tr, 'tr', info);
  const byKey = Object.fromEntries(rows.map((r) => [r.key, r.value]));
  assert.equal(byKey.format, 'WOFF2, TrueType');
  assert.equal(byKey.weight, 'Normal (400)');
  assert.equal(byKey.axes, 'wght 100-900');
  assert.equal(byKey.glyphs, '652');
  assert.equal(fontTitle(info, 'x.woff2'), 'Go Regular');
  assert.equal(fontTitle({ family: 'Go', style: 'Bold' }, 'x.ttf'), 'Go Bold');
  assert.equal(fontTitle(null, 'x.ttf'), 'x.ttf');
});

test('a picture comes over as base64, and only a type a browser shows is shown', () => {
  assert.deepEqual([...base64ToBytes('iVBORw==')], [0x89, 0x50, 0x4e, 0x47]);
  assert.ok(shownType('image/webp'));
  assert.ok(!shownType('image/tiff'));
  assert.ok(!shownType('text/html'));
  assert.equal(programOf({ kind: 'pixelmator' }), 'Pixelmator Pro');
  assert.equal(programOf({ kind: 'psd', program: 'Adobe Photoshop' }), 'Adobe Photoshop');
});
