// Text presentation options for a competition article.
// One options object drives rendering, counting and answer comparison so the
// three can never drift apart. Stored per competition as JSON, which keeps
// adding new toggles a schema-free change.
export type TextOptions = {
  pinyin: boolean;
  stripSymbols: boolean;
  unifySymbols: boolean;
  stripBreaks: boolean;
  segmentWords: boolean;
};

export type TextOptionKey = keyof TextOptions;

export const DEFAULT_TEXT_OPTIONS: TextOptions = {
  pinyin: true,
  stripSymbols: false,
  unifySymbols: false,
  stripBreaks: false,
  segmentWords: false,
};

// Labels are deliberately four characters each and hints eight: the menu is a
// fixed-width list, and uneven labels are what makes such lists look untidy.
export const TEXT_OPTION_ITEMS: ReadonlyArray<{
  key: TextOptionKey;
  label: string;
  hint: string;
}> = [
  { key: "pinyin", label: "拼音提示", hint: "汉字上方显示拼音" },
  { key: "stripSymbols", label: "去除符号", hint: "隐藏标点只练文字" },
  { key: "unifySymbols", label: "符号统一", hint: "全半角标点同判定" },
  { key: "stripBreaks", label: "去除换行", hint: "合并段落去掉缩进" },
  { key: "segmentWords", label: "分词输入", hint: "按词分组显示输入" },
];

export function countEnabledOptions(options: TextOptions): number {
  return TEXT_OPTION_ITEMS.reduce(
    (total, item) => total + Number(options[item.key]),
    0,
  );
}

export function enabledOptionLabels(options: TextOptions): string[] {
  return TEXT_OPTION_ITEMS.filter((item) => options[item.key]).map(
    (item) => item.label,
  );
}

/** Merge a server payload (new text_options plus legacy pinyin_enabled) into a
 *  complete options object. Older servers/pages only know pinyin_enabled. */
export function resolveTextOptions(
  competition?: {
    text_options?: Partial<TextOptions> | null;
    pinyin_enabled?: boolean;
  } | null,
): TextOptions {
  if (!competition) return DEFAULT_TEXT_OPTIONS;
  const legacyPinyin = competition.pinyin_enabled !== false;
  const stored = competition.text_options;
  if (!stored || typeof stored !== "object") {
    return { ...DEFAULT_TEXT_OPTIONS, pinyin: legacyPinyin };
  }
  return {
    pinyin: stored.pinyin === undefined ? legacyPinyin : !!stored.pinyin,
    stripSymbols: !!stored.stripSymbols,
    unifySymbols: !!stored.unifySymbols,
    stripBreaks: !!stored.stripBreaks,
    segmentWords: !!stored.segmentWords,
  };
}

// Punctuation and symbols (Unicode P/S). Spaces and line breaks are not
// symbols; "去除换行" owns those.
const SYMBOL_PATTERN = /[\p{P}\p{S}]/gu;

/** Remove paragraph indentation and line breaks, keeping ASCII words apart. */
function collapseBreaks(text: string): string {
  let out = "";
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    // Chinese text joins seamlessly; ASCII words need a space to stay readable.
    if (out && /[0-9A-Za-z]$/.test(out) && /^[0-9A-Za-z]/.test(line)) out += " ";
    out += line;
  }
  return out;
}

export function applyTextOptions(content: string, options: TextOptions): string {
  let text = content.replace(/\r\n?/g, "\n");
  if (options.stripBreaks) text = collapseBreaks(text);
  if (options.stripSymbols) text = text.replace(SYMBOL_PATTERN, "");
  return text;
}

// Full-width/half-width pairs that Chinese input methods routinely mix up.
// Judging treats both members as the same character when "符号统一" is on;
// the article text itself is never rewritten, so numbers such as 1.5 and
// English such as C++ stay intact.
const SYMBOL_EQUIVALENTS: ReadonlyArray<readonly [string, string]> = [
  ["，", ","],
  ["。", "."],
  ["、", ","],
  ["！", "!"],
  ["？", "?"],
  ["；", ";"],
  ["：", ":"],
  ["（", "("],
  ["）", ")"],
  ["【", "["],
  ["】", "]"],
  ["｛", "{"],
  ["｝", "}"],
  ["“", "\""],
  ["”", "\""],
  ["‘", "'"],
  ["’", "'"],
  ["～", "~"],
];

const SYMBOL_CANONICAL: Map<string, string> = (() => {
  const map = new Map<string, string>();
  for (const [full, half] of SYMBOL_EQUIVALENTS) map.set(full, half);
  for (let code = 0xff10; code <= 0xff19; code++)
    map.set(String.fromCharCode(code), String.fromCharCode(code - 0xfee0));
  for (let code = 0xff21; code <= 0xff3a; code++)
    map.set(String.fromCharCode(code), String.fromCharCode(code - 0xfee0));
  for (let code = 0xff41; code <= 0xff5a; code++)
    map.set(String.fromCharCode(code), String.fromCharCode(code - 0xfee0));
  return map;
})();

function canonicalSymbol(char: string): string {
  return SYMBOL_CANONICAL.get(char) ?? char;
}

/** Answer comparison used by counting and cell colouring. Strict by default;
 *  with "符号统一" on, full-width and half-width variants are interchangeable. */
export function equivalentChar(
  expected: string,
  typed: string,
  options: TextOptions,
): boolean {
  if (expected === typed) return true;
  if (!options.unifySymbols) return false;
  if (expected.length !== 1 || typed.length !== 1) return false;
  return canonicalSymbol(expected) === canonicalSymbol(typed);
}

/** True when the character is rendered with a pinyin hint. */
export function isPinyinChar(char: string): boolean {
  return !/[\p{P}\p{S}\s]/u.test(char);
}
