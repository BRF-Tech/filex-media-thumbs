/**
 * Media Thumb Engine's preview page. filex opens it in place of its own
 * preview for a design file (the `design` view) or a font (the `font` view),
 * in a sandboxed frame with no network: everything goes through the bridge
 * (@brftech/filex-app-ui, vendored).
 *
 *  - A design file: the module reads the picture the file carries and what
 *    the file says about itself (`fx.call('preview')`, internal/app), and the
 *    page shows them with a sentence on where the picture comes from.
 *  - A font: the browser draws it (`FontFace`, from the file's bytes - the
 *    browser checks every web font first), and the module reads its names,
 *    weight, axes and characters (`fx.call('font')`), WOFF2 included.
 *
 * Every text the page shows is built with textContent: a file's name and a
 * font's names are the file's words, never markup.
 */
import { connect } from './vendor/filex-app-ui.js';
import { pickLanguage, translator } from './lib/i18n.js';
import {
  CHARS_FIRST,
  CHARS_MAX,
  SAMPLE_SIZES,
  base64ToBytes,
  captionOf,
  codepointLabel,
  defaultSample,
  designFacts,
  explanationOf,
  fontFacts,
  fontTitle,
  printableCodepoints,
  problemText,
  programOf,
  shownType,
  variationSettings,
  weightName,
  weightRows,
} from './lib/view.js';

const root = document.getElementById('app');

/** What the page shows; render() draws it, again on a change of language. */
const state = {
  mode: 'loading', // loading | outside | design | font
  file: null,
  answer: null, // the module's answer
  callError: '', // a refusal of the module's call, in the reader's words
  pictureUrl: '',
  family: '', // the loaded font's name on this page
  fontError: '',
  sample: '',
  sampleTouched: false,
  showAll: false,
};

let t = translator('en');
let lang = 'en';

/** A small element builder: el('p', {class: 'x'}, 'text', child). */
function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k === 'style') Object.assign(node.style, v);
    else if (k === 'text') node.textContent = v;
    else node.setAttribute(k, v === true ? '' : String(v));
  }
  for (const c of children) {
    if (c === undefined || c === null || c === false) continue;
    node.append(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return node;
}

function setLanguage(locale) {
  lang = pickLanguage(locale);
  t = translator(locale);
}

function render() {
  root.replaceChildren();
  switch (state.mode) {
    case 'loading':
      // Before filex has answered, the reader's language is not known yet:
      // the spinner alone.
      root.append(el('div', { class: 'mt-status mt-status--busy', role: 'status', 'aria-busy': 'true' }, fx ? t('app.loading') : ''));
      return;
    case 'outside':
      root.append(el('div', { class: 'mt-status', text: t('app.outside') }));
      return;
    case 'design':
      root.append(renderDesign());
      return;
    case 'font':
      root.append(renderFont());
  }
}

function factList(rows, extraClass) {
  const dl = el('dl', { class: `mt-facts${extraClass ? ` ${extraClass}` : ''}` });
  for (const r of rows) {
    dl.append(el('div', { class: 'mt-fact', 'data-fact': r.key }, el('dt', { text: r.label }), el('dd', { text: r.value })));
  }
  return dl;
}

// ── a design file ──────────────────────────────────────────────────────

function renderDesign() {
  const a = state.answer || {};
  const program = programOf(a);
  const main = el('main', { class: 'mt mt--design', 'data-kind': a.kind || '' });
  main.append(
    el('header', { class: 'mt-head' }, el('h1', { class: 'mt-title', text: state.file ? state.file.name : '' }), el('p', { class: 'mt-kind', text: program })),
  );
  const body = el('div', { class: 'mt-body' });
  if (state.pictureUrl && a.picture) {
    const caption = captionOf(t, a.picture, program);
    body.append(
      el(
        'figure',
        { class: 'mt-figure' },
        el('div', { class: 'mt-stage' }, el('img', { class: 'mt-picture', src: state.pictureUrl, alt: caption, 'data-testid': 'design-picture' })),
        el('figcaption', { class: 'mt-caption' }, caption, a.picture.drawn ? ` ${t('design.caption.drawn')}` : ''),
      ),
    );
  } else {
    const text = state.callError || problemText(t, a.problem, program, a.kind);
    body.append(el('div', { class: 'mt-problem', role: 'status', 'data-problem': a.problem || 'error' }, el('p', { text })));
  }
  const about = el('section', { class: 'mt-about' });
  if (a.problem !== 'cython') {
    const explain = explanationOf(t, a.kind);
    if (explain) about.append(el('p', { class: 'mt-explain', text: explain }));
  }
  const rows = designFacts(t, lang, a.facts);
  if (rows.length) about.append(factList(rows));
  if (program && a.problem !== 'cython') about.append(el('p', { class: 'mt-hint', text: t('design.download_hint', { program }) }));
  body.append(about);
  main.append(body);
  return main;
}

async function loadDesign() {
  let answer;
  try {
    answer = await fx.call('preview');
  } catch (err) {
    console.warn('[media-thumbs] preview call failed:', err);
    state.answer = { kind: '', problem: 'damaged' };
    state.callError = callErrorText(err);
    state.mode = 'design';
    render();
    return;
  }
  state.answer = answer || {};
  if (state.answer.detail) console.info('[media-thumbs]', state.answer.problem || 'preview', state.answer.detail);
  const pic = state.answer.picture;
  if (pic && shownType(pic.mime) && pic.data) {
    try {
      state.pictureUrl = URL.createObjectURL(new Blob([base64ToBytes(pic.data)], { type: pic.mime }));
    } catch (err) {
      console.warn('[media-thumbs] the picture could not be read:', err);
      state.answer.problem = 'damaged';
    }
  }
  state.mode = 'design';
  render();
}

/** The sentence for a refused call: the module's own (code `failed`), else
 *  one of the page's. */
function callErrorText(err) {
  const code = err && err.code;
  const message = err && err.message;
  if (code === 'failed' && message && message !== 'failed') return message;
  if (code === 'unavailable' || code === 'not_granted') return t('problem.unavailable');
  return t('problem.damaged');
}

// ── a font ─────────────────────────────────────────────────────────────

/** The spans that show the sample, updated as the person types. */
let sampleNodes = [];

function useFont(extra) {
  const style = { fontFamily: `"${state.family}", system-ui, sans-serif` };
  return Object.assign(style, extra || {});
}

function renderFont() {
  const a = state.answer || {};
  const info = a.info || null;
  const main = el('main', { class: 'mt mt--font' });
  const titleText = fontTitle(info, state.file ? state.file.name : '');
  const head = el('header', { class: 'mt-head' });
  head.append(el('h1', { class: `mt-title${state.family ? ' mt-usefont' : ''}`, style: state.family ? useFont() : null, text: titleText }));
  if (info) head.append(factList(fontFacts(t, lang, info), 'mt-facts--inline'));
  if (info && info.format === 'collection') head.append(el('p', { class: 'mt-note', text: t('font.collection_note') }));
  main.append(head);

  if (!state.family) {
    const text = state.fontError || state.callError || (a.problem ? problemText(t, a.problem, '', '') : t('font.unreadable'));
    main.append(el('div', { class: 'mt-problem', role: 'status' }, el('p', { text })));
    return main;
  }
  if (a.problem) {
    main.append(el('p', { class: 'mt-note', text: problemText(t, a.problem, '', '') }));
  }

  if (!state.sampleTouched) state.sample = defaultSample(t, info ? info.codepoints : null);
  sampleNodes = [];
  const input = el('input', { type: 'text', class: 'mt-sample-input', dir: 'auto', spellcheck: 'false', 'data-testid': 'font-sample' });
  input.value = state.sample;
  input.addEventListener('input', () => {
    state.sample = input.value;
    state.sampleTouched = true;
    for (const n of sampleNodes) n.textContent = state.sample;
  });
  main.append(el('label', { class: 'mt-sample-label' }, el('span', { text: t('font.sample_label') }), input));

  const sampleSpan = (style) => {
    const s = el('span', { class: 'mt-text mt-usefont', dir: 'auto', style: useFont(style), text: state.sample });
    sampleNodes.push(s);
    return s;
  };

  const sizes = el('section', { class: 'mt-section' }, el('h2', { text: t('font.sizes') }));
  for (const size of SAMPLE_SIZES) {
    sizes.append(el('div', { class: 'mt-line' }, el('span', { class: 'mt-tag', text: t('font.size_px', { size }) }), sampleSpan({ fontSize: `${size}px` })));
  }
  main.append(sizes);

  const rows = weightRows(info);
  const named = rows.some((r) => r.label);
  const variable = rows.some((r) => r.coords);
  const weights = el('section', { class: 'mt-section' }, el('h2', { text: named ? t('font.instances') : variable ? t('font.weights') : t('font.weight_one') }));
  for (const r of rows) {
    const label = r.label || `${weightName(t, r.weight)} ${r.weight}`;
    const style = { fontSize: '28px' };
    const settings = variationSettings(r.coords);
    if (settings) style.fontVariationSettings = settings;
    weights.append(el('div', { class: 'mt-line' }, el('span', { class: 'mt-tag', text: label }), sampleSpan(style)));
  }
  main.append(weights);

  const all = printableCodepoints(info ? info.codepoints : null);
  const chars = el('section', { class: 'mt-section' });
  if (all.length) {
    const total = info.codepoints_total || all.length;
    chars.append(el('h2', { text: t('font.characters', { total: new Intl.NumberFormat(lang === 'tr' ? 'tr-TR' : 'en-US').format(total) }) }));
    const list = el('ul', { class: 'mt-chars mt-usefont', style: useFont(), 'data-testid': 'font-chars' });
    for (const c of all.slice(0, state.showAll ? CHARS_MAX : CHARS_FIRST)) {
      list.append(el('li', { title: codepointLabel(c), text: String.fromCodePoint(c) }));
    }
    chars.append(list);
    if (!state.showAll && all.length > CHARS_FIRST) {
      const more = el('button', { type: 'button', class: 'mt-button', text: t('font.show_all') });
      more.addEventListener('click', () => {
        state.showAll = true;
        render();
      });
      chars.append(more);
    }
    if (state.showAll && all.length > CHARS_MAX) chars.append(el('p', { class: 'mt-note', text: t('font.first_chars', { shown: CHARS_MAX }) }));
  } else {
    chars.append(el('p', { class: 'mt-note', text: t('font.no_chars') }));
  }
  main.append(chars);
  return main;
}

async function loadFont() {
  const [call, bytes] = await Promise.allSettled([fx.call('font'), fx.open().then((f) => f.bytes())]);
  if (call.status === 'fulfilled') {
    state.answer = call.value || {};
    if (state.answer.detail) console.info('[media-thumbs]', state.answer.problem || 'font', state.answer.detail);
  } else {
    console.warn('[media-thumbs] font call failed:', call.reason);
    state.answer = {};
    state.callError = callErrorText(call.reason);
  }
  if (bytes.status === 'fulfilled' && typeof FontFace !== 'undefined') {
    // A name no other font on the page has: the font never stands in for
    // the page's own text.
    const name = `mt-font-${Math.random().toString(36).slice(2, 10)}`;
    try {
      const face = await new FontFace(name, bytes.value).load();
      document.fonts.add(face);
      state.family = name;
    } catch (err) {
      // The browser's own words (an OpenType Sanitizer message) go to the
      // console; the person gets a sentence.
      console.warn('[media-thumbs] the browser did not take the font:', err);
      state.fontError = t('font.unreadable');
    }
  } else {
    if (bytes.status === 'rejected') console.warn('[media-thumbs] the font could not be read:', bytes.reason);
    state.fontError = bytes.status === 'rejected' ? callErrorText(bytes.reason) : t('font.unreadable');
  }
  state.mode = 'font';
  render();
}

// ── start ──────────────────────────────────────────────────────────────

let fx;

async function start() {
  render();
  try {
    fx = await connect();
  } catch {
    state.mode = 'outside';
    render();
    return;
  }
  setLanguage(fx.session.locale);
  fx.on('locale', (d) => {
    setLanguage(d && d.locale);
    if (!state.sampleTouched) state.sample = '';
    render();
  });
  state.file = (fx.session.files || [])[0] || null;
  const view = fx.session.view && fx.session.view.id;
  const isFont = view === 'font' || (view !== 'design' && state.file && ['ttf', 'otf', 'woff', 'woff2'].includes(state.file.ext));
  document.title = `${state.file ? state.file.name : ''} - ${isFont ? t('app.font_title') : t('app.design_title')}`;
  render();
  if (isFont) await loadFont();
  else await loadDesign();
}

window.addEventListener('pagehide', () => {
  if (state.pictureUrl) URL.revokeObjectURL(state.pictureUrl);
});

start();
