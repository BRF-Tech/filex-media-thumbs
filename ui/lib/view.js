/**
 * What the page shows, worked out from the module's answers - plain
 * functions with no DOM in them, so test/view.test.js reads them in Node.
 *
 * The module (internal/app) answers `preview` for a design file and `font`
 * for a font; this turns those answers into sentences, rows of facts, the
 * weights a font is shown at and the characters it draws.
 */

/** The characters drawn before "Show all", and at most after it. */
export const CHARS_FIRST = 256;
export const CHARS_MAX = 4096;

/** The sizes a font's sample is drawn at, in CSS pixels. */
export const SAMPLE_SIZES = [12, 16, 24, 32, 48, 72];

/** The program that writes a design file, by the kind the module says. */
export function programOf(answer) {
  if (answer && answer.program) return answer.program;
  switch (answer && answer.kind) {
    case 'psd':
      return 'Adobe Photoshop';
    case 'affinity':
      return 'Affinity';
    case 'pixelmator':
      return 'Pixelmator Pro';
    default:
      return '';
  }
}

/** The sentence for a design file without a picture, or a font whose facts
 *  could not be read. */
export function problemText(t, problem, program, kind) {
  switch (problem) {
    case 'no_preview':
      return kind === 'psd' ? t('problem.no_preview_psd') : t('problem.no_preview', { program });
    case 'encrypted':
    case 'too_large':
    case 'timeout':
    case 'cython':
    case 'not_a_font':
    case 'damaged':
    case 'unavailable':
      return t(`problem.${problem}`);
    case 'not_this_format':
      return t('problem.not_this_format', { program });
    default:
      return t('problem.damaged');
  }
}

/** The caption under a design file's picture. */
export function captionOf(t, picture, program) {
  if (!picture) return '';
  const vars = { program, width: picture.width, height: picture.height };
  if (picture.source === 'merged') return t('design.caption.merged', vars);
  if (picture.source === 'thumbnail') return t('design.caption.thumbnail', vars);
  return t('design.caption.preview', vars);
}

/** The explanation of where a design file's picture comes from. */
export function explanationOf(t, kind) {
  switch (kind) {
    case 'psd':
      return t('design.explain.psd');
    case 'affinity':
      return t('design.explain.affinity');
    case 'pixelmator':
      return t('design.explain.pixelmator');
    default:
      return '';
  }
}

const AFFINITY_DOCUMENTS = {
  af: 'Affinity',
  afphoto: 'Affinity Photo',
  afdesign: 'Affinity Designer',
  afpub: 'Affinity Publisher',
};

/** A number in the reader's language. */
function num(lang, n) {
  try {
    return new Intl.NumberFormat(lang === 'tr' ? 'tr-TR' : 'en-US').format(n);
  } catch {
    return String(n);
  }
}

/** The rows of facts a design file states, in the order they are read. */
export function designFacts(t, lang, facts) {
  const f = facts || {};
  const rows = [];
  const add = (key, label, value) => {
    if (value !== undefined && value !== null && String(value) !== '') rows.push({ key, label, value: String(value) });
  };
  if (f.format) add('format', t('fact.format'), t(`format.${f.format}`));
  if (f.document && AFFINITY_DOCUMENTS[f.document]) add('document', t('fact.document'), AFFINITY_DOCUMENTS[f.document]);
  if (f.width && f.height) add('size', t('fact.size'), t('fact.size_value', { width: num(lang, f.width), height: num(lang, f.height) }));
  if (f.mode) add('mode', t('fact.mode'), t(`mode.${f.mode}`));
  if (f.depth) add('depth', t('fact.depth'), t('fact.depth_value', { depth: f.depth }));
  if (f.channels) add('channels', t('fact.channels'), num(lang, f.channels));
  if (typeof f.layers === 'number') add('layers', t('fact.layers'), num(lang, f.layers));
  if (f.merged === 'real' || f.merged === 'not_real') add('merged', t('fact.merged'), t(`fact.merged.${f.merged}`));
  if (f.container) {
    add('container', t('fact.container'), f.generation ? t('fact.container_value', { version: f.container, generation: f.generation }) : String(f.container));
  }
  if (typeof f.members === 'number') add('members', t('fact.members'), num(lang, f.members));
  if (f.preview_member) add('preview_member', t('fact.preview_member'), f.preview_member);
  return rows;
}

/** The weight a weight class is called (the nearest hundred). */
export function weightName(t, weight) {
  const w = Math.min(900, Math.max(100, Math.round(Number(weight || 400) / 100) * 100));
  return t(`weight.${w}`);
}

/** The rows of facts a font states. */
export function fontFacts(t, lang, info) {
  const i = info || {};
  const rows = [];
  const add = (key, label, value) => {
    if (value !== undefined && value !== null && String(value) !== '') rows.push({ key, label, value: String(value) });
  };
  add('style', t('font.fact.style'), i.style);
  if (i.weight) add('weight', t('font.fact.weight'), `${weightName(t, i.weight)} (${i.weight})`);
  if (i.italic) add('italic', t('font.fact.italic'), t('font.yes'));
  if (Array.isArray(i.axes) && i.axes.length) {
    add('axes', t('font.fact.axes'), i.axes.map((a) => `${a.tag} ${a.min}-${a.max}`).join(', '));
  }
  if (i.glyphs) add('glyphs', t('font.fact.glyphs'), num(lang, i.glyphs));
  const format = i.format;
  if (format) {
    let value = t(`font.format.${format}`);
    if ((format === 'woff' || format === 'woff2' || format === 'collection') && i.outlines) {
      value += `, ${t(`font.format.${i.outlines}`)}`;
    }
    add('format', t('font.fact.format'), value);
  }
  add('version', t('font.fact.version'), i.version);
  add('designer', t('font.fact.designer'), i.designer);
  add('maker', t('font.fact.maker'), i.maker);
  add('license', t('font.fact.license'), i.license);
  add('copyright', t('font.fact.copyright'), i.copyright);
  return rows;
}

/** The title a font is shown under: its full name, else family and style. */
export function fontTitle(info, fileName) {
  const i = info || {};
  return i.full_name || [i.family, i.style].filter(Boolean).join(' ') || fileName || '';
}

/**
 * The weights a font is shown at. A variable font: its named instances
 * when it has them (each with every axis it sets), else its weight axis at
 * every hundred, its ends included. A static font: the one weight it is -
 * the browser would only fake another.
 */
export function weightRows(info) {
  const i = info || {};
  if (Array.isArray(i.instances) && i.instances.length) {
    return i.instances.slice(0, 24).map((x) => ({ label: x.name, coords: x.coords || {}, weight: (x.coords || {}).wght }));
  }
  const axis = (i.axes || []).find((a) => a.tag === 'wght');
  if (axis && axis.max > axis.min) {
    const stops = new Set([Math.round(axis.min), Math.round(axis.max)]);
    for (let w = Math.ceil(axis.min / 100) * 100; w < axis.max; w += 100) stops.add(w);
    return [...stops]
      .sort((a, b) => a - b)
      .slice(0, 12)
      .map((w) => ({ label: '', coords: { wght: w }, weight: w }));
  }
  return [{ label: '', coords: null, weight: i.weight || 400 }];
}

/** The CSS font-variation-settings for an instance's coordinates. */
export function variationSettings(coords) {
  if (!coords) return '';
  return Object.entries(coords)
    .filter(([tag, v]) => /^[\x20-\x7e]{4}$/.test(tag) && !/["\\]/.test(tag) && Number.isFinite(v))
    .map(([tag, v]) => `"${tag}" ${v}`)
    .join(', ');
}

/** The characters worth showing in the grid: no controls, no spaces, no
 *  surrogates. */
export function printableCodepoints(codepoints) {
  return (codepoints || []).filter(
    (c) => c > 0x20 && !(c >= 0x7f && c <= 0xa0) && c !== 0xad && !(c >= 0xd800 && c <= 0xdfff) && c <= 0x10ffff,
  );
}

/** Whether the font draws every character of text (an unread map says yes). */
export function fontDraws(codepoints, text) {
  if (!Array.isArray(codepoints)) return true;
  const have = new Set(codepoints);
  for (const ch of text) {
    if (ch.trim() === '') continue;
    if (!have.has(ch.codePointAt(0))) return false;
  }
  return true;
}

/** The sample a font is first shown with: the reader's sentence when the
 *  font draws it, else its own first characters. */
export function defaultSample(t, codepoints) {
  const preferred = t('font.sample_default');
  if (fontDraws(codepoints, preferred)) return preferred;
  const own = printableCodepoints(codepoints).slice(0, 40);
  return own.length ? String.fromCodePoint(...own) : preferred;
}

/** U+0041. */
export function codepointLabel(c) {
  return `U+${c.toString(16).toUpperCase().padStart(4, '0')}`;
}

/** The bytes of a base64 string (a picture in the module's answer). */
export function base64ToBytes(s) {
  const bin = atob(String(s || ''));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/** The picture types the page shows. */
const SHOWN = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif']);

/** Whether a picture's type is one the page shows (and its blob may carry). */
export function shownType(mime) {
  return SHOWN.has(String(mime || ''));
}
