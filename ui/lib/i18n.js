/**
 * The page's words, in the two languages the app speaks (filex-app.json
 * `languages`). Every key is in both; test/i18n.test.js holds them together
 * and keeps the Turkish in its own letters.
 *
 * A reader whose filex speaks another language reads English, as filex
 * itself falls back to English.
 */

export const STRINGS = {
  en: {
    'app.loading': 'Loading…',
    'app.outside': 'This preview runs inside filex.',
    'app.design_title': 'Design preview',
    'app.font_title': 'Font preview',

    'design.caption.preview': 'The preview {program} saved inside the file, {width} × {height} pixels.',
    'design.caption.thumbnail': 'The thumbnail {program} saved inside the file, {width} × {height} pixels.',
    'design.caption.merged': 'The merged image of the document, drawn at {width} × {height} pixels.',
    'design.caption.drawn': 'Scaled down to fit this page.',
    'design.explain.psd':
      'Photoshop saves a flattened copy of the whole document (the merged image) when "Maximize Compatibility" is on, and a small thumbnail at the start of the file. This preview shows the merged image, or the thumbnail when there is no merged image. The layers are not opened.',
    'design.explain.affinity':
      'Affinity saves a flattened PNG picture of the document inside the file, for file browsers. This preview shows that picture; the document itself is not opened.',
    'design.explain.pixelmator':
      'A Pixelmator Pro document is a ZIP file, and Pixelmator Pro keeps a preview of the document in it for file browsers. This preview shows that picture; the document itself is not opened.',
    'design.download_hint': 'To edit the document, download it and open it in {program}.',

    'problem.no_preview': 'This {program} file holds no preview that can be shown here. Download it to open it in {program}.',
    'problem.no_preview_psd': 'This Photoshop file holds neither a merged image nor a thumbnail that can be shown here. Saving it with "Maximize Compatibility" on gives the next copy one.',
    'problem.encrypted': 'The preview inside this file is encrypted, so it cannot be shown here.',
    'problem.too_large': 'The preview inside this file is too large to show here. Download the file to open it.',
    'problem.timeout': 'Reading the file took too long. Try again in a moment.',
    'problem.cython':
      'This .pxd file is text: a Cython declaration file, not a Pixelmator Pro document. Open it with the built-in viewer (Open with).',
    'problem.not_this_format': 'This file is not a {program} file, though its name says so.',
    'problem.not_a_font': 'This file is not a font, though its name says so.',
    'problem.damaged': 'This file could not be read. It may be damaged.',
    'problem.unavailable': 'The file could not be reached. Try again in a moment.',

    'fact.format': 'Format',
    'fact.size': 'Document size',
    'fact.size_value': '{width} × {height} pixels',
    'fact.mode': 'Color mode',
    'fact.depth': 'Bit depth',
    'fact.depth_value': '{depth} bits per channel',
    'fact.channels': 'Channels',
    'fact.layers': 'Layers',
    'fact.merged': 'Merged image',
    'fact.merged.real': 'Saved in the file',
    'fact.merged.not_real': 'Not saved ("Maximize Compatibility" was off)',
    'fact.document': 'Document',
    'fact.container': 'Container version',
    'fact.container_value': '{version} (Affinity {generation})',
    'fact.members': 'Items in the ZIP',
    'fact.preview_member': 'Preview inside the file',

    'format.psd': 'Photoshop document (PSD)',
    'format.psb': 'Photoshop large document (PSB)',
    'format.affinity': 'Affinity document',
    'format.pixelmator': 'Pixelmator Pro document',

    'mode.bitmap': 'Bitmap',
    'mode.grayscale': 'Grayscale',
    'mode.indexed': 'Indexed color',
    'mode.rgb': 'RGB color',
    'mode.cmyk': 'CMYK color',
    'mode.multichannel': 'Multichannel',
    'mode.duotone': 'Duotone',
    'mode.lab': 'Lab color',

    'font.sample_label': 'Sample text',
    'font.sample_default': 'The quick brown fox jumps over the lazy dog',
    'font.sizes': 'Sizes',
    'font.size_px': '{size} px',
    'font.weights': 'Weights',
    'font.instances': 'Named styles',
    'font.weight_one': 'Its weight',
    'font.characters': 'Characters ({total})',
    'font.show_all': 'Show all characters',
    'font.first_chars': 'The first {shown} are shown.',
    'font.no_chars': 'The list of characters could not be read from this font.',
    'font.unreadable': 'Your browser could not draw this font. It may be damaged, or of a kind the browser does not take.',
    'font.collection_note': 'This is a font collection; the preview shows its first font.',
    'font.fact.style': 'Style',
    'font.fact.weight': 'Weight',
    'font.fact.width': 'Width',
    'font.fact.italic': 'Italic',
    'font.fact.axes': 'Variable axes',
    'font.fact.glyphs': 'Glyphs',
    'font.fact.format': 'Format',
    'font.fact.version': 'Version',
    'font.fact.designer': 'Designer',
    'font.fact.maker': 'Foundry',
    'font.fact.license': 'License',
    'font.fact.copyright': 'Copyright',
    'font.yes': 'Yes',
    'font.format.truetype': 'TrueType',
    'font.format.opentype': 'OpenType (PostScript outlines)',
    'font.format.collection': 'Font collection',
    'font.format.woff': 'WOFF',
    'font.format.woff2': 'WOFF2',

    'weight.100': 'Thin',
    'weight.200': 'Extra Light',
    'weight.300': 'Light',
    'weight.400': 'Regular',
    'weight.500': 'Medium',
    'weight.600': 'Semi Bold',
    'weight.700': 'Bold',
    'weight.800': 'Extra Bold',
    'weight.900': 'Black',
  },
  tr: {
    'app.loading': 'Yükleniyor…',
    'app.outside': 'Bu önizleme filex içinde çalışır.',
    'app.design_title': 'Tasarım önizlemesi',
    'app.font_title': 'Font önizlemesi',

    'design.caption.preview': '{program} programının dosyanın içine kaydettiği önizleme, {width} × {height} piksel.',
    'design.caption.thumbnail': '{program} programının dosyanın içine kaydettiği küçük resim, {width} × {height} piksel.',
    'design.caption.merged': 'Belgenin birleştirilmiş görüntüsü, {width} × {height} piksel olarak çizildi.',
    'design.caption.drawn': 'Bu sayfaya sığması için küçültüldü.',
    'design.explain.psd':
      'Photoshop, "Uyumluluğu En Üst Düzeye Çıkar" (Maximize Compatibility) açıkken belgenin düzleştirilmiş bir kopyasını (birleştirilmiş görüntü), dosyanın başına da küçük bir resim kaydeder. Bu önizleme birleştirilmiş görüntüyü, o yoksa küçük resmi gösterir. Katmanlar açılmaz.',
    'design.explain.affinity':
      'Affinity, dosya tarayıcıları için belgenin düzleştirilmiş bir PNG resmini dosyanın içine kaydeder. Bu önizleme o resmi gösterir; belgenin kendisi açılmaz.',
    'design.explain.pixelmator':
      'Bir Pixelmator Pro belgesi aslında bir ZIP dosyasıdır; Pixelmator Pro, dosya tarayıcıları için belgenin bir önizlemesini bunun içinde tutar. Bu önizleme o resmi gösterir; belgenin kendisi açılmaz.',
    'design.download_hint': 'Belgeyi düzenlemek için indirip {program} ile açın.',

    'problem.no_preview': 'Bu {program} dosyası burada gösterilebilecek bir önizleme taşımıyor. {program} ile açmak için dosyayı indirin.',
    'problem.no_preview_psd': 'Bu Photoshop dosyası burada gösterilebilecek ne birleştirilmiş bir görüntü ne de küçük bir resim taşıyor. "Uyumluluğu En Üst Düzeye Çıkar" açıkken kaydedilen bir sonraki kopyada önizleme olur.',
    'problem.encrypted': 'Bu dosyanın içindeki önizleme şifreli; bu yüzden burada gösterilemiyor.',
    'problem.too_large': 'Bu dosyanın içindeki önizleme burada gösterilemeyecek kadar büyük. Açmak için dosyayı indirin.',
    'problem.timeout': 'Dosyayı okumak çok uzun sürdü. Biraz sonra yeniden deneyin.',
    'problem.cython':
      'Bu .pxd dosyası metin: bir Pixelmator Pro belgesi değil, bir Cython bildirim dosyası. Yerleşik görüntüleyiciyle açın (Birlikte aç).',
    'problem.not_this_format': 'Adı öyle söylese de bu dosya bir {program} dosyası değil.',
    'problem.not_a_font': 'Adı öyle söylese de bu dosya bir font değil.',
    'problem.damaged': 'Bu dosya okunamadı. Hasarlı olabilir.',
    'problem.unavailable': 'Dosyaya ulaşılamadı. Biraz sonra yeniden deneyin.',

    'fact.format': 'Biçim',
    'fact.size': 'Belge boyutu',
    'fact.size_value': '{width} × {height} piksel',
    'fact.mode': 'Renk modu',
    'fact.depth': 'Bit derinliği',
    'fact.depth_value': 'Kanal başına {depth} bit',
    'fact.channels': 'Kanallar',
    'fact.layers': 'Katmanlar',
    'fact.merged': 'Birleştirilmiş görüntü',
    'fact.merged.real': 'Dosyada kayıtlı',
    'fact.merged.not_real': 'Kaydedilmemiş ("Uyumluluğu En Üst Düzeye Çıkar" kapalıydı)',
    'fact.document': 'Belge',
    'fact.container': 'Kapsayıcı sürümü',
    'fact.container_value': '{version} (Affinity {generation})',
    'fact.members': 'ZIP içindeki öğeler',
    'fact.preview_member': 'Dosyadaki önizleme',

    'format.psd': 'Photoshop belgesi (PSD)',
    'format.psb': 'Photoshop büyük belgesi (PSB)',
    'format.affinity': 'Affinity belgesi',
    'format.pixelmator': 'Pixelmator Pro belgesi',

    'mode.bitmap': 'Bit eşlem',
    'mode.grayscale': 'Gri tonlama',
    'mode.indexed': 'Dizinlenmiş renk',
    'mode.rgb': 'RGB renk',
    'mode.cmyk': 'CMYK renk',
    'mode.multichannel': 'Çok kanallı',
    'mode.duotone': 'Çift ton',
    'mode.lab': 'Lab renk',

    'font.sample_label': 'Örnek metin',
    'font.sample_default': 'Pijamalı hasta yağız şoföre çabucak güvendi',
    'font.sizes': 'Boyutlar',
    'font.size_px': '{size} px',
    'font.weights': 'Ağırlıklar',
    'font.instances': 'Adlandırılmış stiller',
    'font.weight_one': 'Ağırlığı',
    'font.characters': 'Karakterler ({total})',
    'font.show_all': 'Tüm karakterleri göster',
    'font.first_chars': 'İlk {shown} karakter gösteriliyor.',
    'font.no_chars': 'Bu fontun karakter listesi okunamadı.',
    'font.unreadable': 'Tarayıcınız bu fontu çizemedi. Font hasarlı olabilir ya da tarayıcının kabul etmediği bir türde olabilir.',
    'font.collection_note': 'Bu bir font koleksiyonu; önizleme ilk fontunu gösteriyor.',
    'font.fact.style': 'Stil',
    'font.fact.weight': 'Ağırlık',
    'font.fact.width': 'Genişlik',
    'font.fact.italic': 'İtalik',
    'font.fact.axes': 'Değişken eksenler',
    'font.fact.glyphs': 'Glifler',
    'font.fact.format': 'Biçim',
    'font.fact.version': 'Sürüm',
    'font.fact.designer': 'Tasarımcı',
    'font.fact.maker': 'Üretici',
    'font.fact.license': 'Lisans',
    'font.fact.copyright': 'Telif hakkı',
    'font.yes': 'Evet',
    'font.format.truetype': 'TrueType',
    'font.format.opentype': 'OpenType (PostScript çizgileri)',
    'font.format.collection': 'Font koleksiyonu',
    'font.format.woff': 'WOFF',
    'font.format.woff2': 'WOFF2',

    'weight.100': 'İnce',
    'weight.200': 'Çok Hafif',
    'weight.300': 'Hafif',
    'weight.400': 'Normal',
    'weight.500': 'Orta',
    'weight.600': 'Yarı Kalın',
    'weight.700': 'Kalın',
    'weight.800': 'Çok Kalın',
    'weight.900': 'Siyah',
  },
};

/** Strings that are the same in both languages on purpose: names, units. */
export const SAME_IN_BOTH = new Set([
  'font.size_px',
  'fact.container_value',
  'font.format.truetype',
  'font.format.woff',
  'font.format.woff2',
]);

/** The page's language for a filex locale (`tr`, `tr-TR`, `en-GB`, `es`). */
export function pickLanguage(locale) {
  const tag = String(locale || '').toLowerCase();
  return tag === 'tr' || tag.startsWith('tr-') ? 'tr' : 'en';
}

/**
 * A translator for a locale: t(key, vars) is the sentence with `{name}`
 * filled in. A key the catalogue does not have is said as itself, so a
 * missing string shows up instead of disappearing.
 */
export function translator(locale) {
  const lang = pickLanguage(locale);
  const table = STRINGS[lang];
  return (key, vars) => {
    const s = table[key] ?? STRINGS.en[key] ?? key;
    if (!vars) return s;
    return s.replace(/\{([a-z_]+)\}/g, (all, name) => (name in vars ? String(vars[name]) : all));
  };
}
