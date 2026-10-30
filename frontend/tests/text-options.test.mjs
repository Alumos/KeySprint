import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// Use the project's compiler so these tests also run on Node 22 without a TS loader.
const source = readFileSync(new URL('../src/lib/textOptions.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } });
const {
  DEFAULT_TEXT_OPTIONS,
  TEXT_OPTION_ITEMS,
  applyTextOptions,
  countEnabledOptions,
  enabledOptionLabels,
  equivalentChar,
  isPinyinChar,
  resolveTextOptions,
} = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('default options leave the article untouched', () => {
  const content = '你好，世界。\n第二段！';
  assert.equal(applyTextOptions(content, DEFAULT_TEXT_OPTIONS), content);
  assert.equal(countEnabledOptions(DEFAULT_TEXT_OPTIONS), 1);
  assert.deepEqual(enabledOptionLabels(DEFAULT_TEXT_OPTIONS), ['拼音提示']);
});

test('every option label and hint is the same length so the list stays aligned', () => {
  assert.equal(TEXT_OPTION_ITEMS.length, 4);
  for (const item of TEXT_OPTION_ITEMS) {
    assert.equal(item.label.length, 4, item.label);
    assert.equal(item.hint.length, 8, item.hint);
  }
});

test('removing symbols strips punctuation but keeps letters, digits and spaces', () => {
  const options = { ...DEFAULT_TEXT_OPTIONS, stripSymbols: true };
  assert.equal(applyTextOptions('你好，世界！', options), '你好世界');
  assert.equal(applyTextOptions('Hello, world! 2026.', options), 'Hello world 2026');
  assert.equal(applyTextOptions('一二\n三，四。', options), '一二\n三四');
});

test('removing line breaks joins paragraphs and drops indentation', () => {
  const options = { ...DEFAULT_TEXT_OPTIONS, stripBreaks: true };
  assert.equal(applyTextOptions('　　第一段，\n\n第二段。', options), '第一段，第二段。');
  assert.equal(applyTextOptions('Hello\nWorld', options), 'Hello World');
  assert.equal(applyTextOptions('中文\nEnglish', options), '中文English');
});

test('symbol unification compares widths without rewriting the article', () => {
  const strict = { ...DEFAULT_TEXT_OPTIONS };
  const unified = { ...DEFAULT_TEXT_OPTIONS, unifySymbols: true };
  assert.equal(equivalentChar('，', ',', strict), false);
  assert.equal(equivalentChar('，', ',', unified), true);
  assert.equal(equivalentChar('。', '.', unified), true);
  assert.equal(equivalentChar('！', '!', unified), true);
  assert.equal(equivalentChar('Ａ', 'A', unified), true);
  assert.equal(equivalentChar('１', '1', unified), true);
  assert.equal(equivalentChar('你', '你', unified), true);
  assert.equal(equivalentChar('你', '好', unified), false);
  assert.equal(applyTextOptions('你好，世界。', unified), '你好，世界。');
});

test('legacy payloads without text_options fall back to pinyin_enabled', () => {
  assert.deepEqual(resolveTextOptions(null), DEFAULT_TEXT_OPTIONS);
  assert.deepEqual(resolveTextOptions({ pinyin_enabled: false }), {
    ...DEFAULT_TEXT_OPTIONS,
    pinyin: false,
  });
  assert.deepEqual(resolveTextOptions({ pinyin_enabled: false, text_options: { pinyin: true } }), {
    ...DEFAULT_TEXT_OPTIONS,
    pinyin: true,
  });
  assert.deepEqual(
    resolveTextOptions({ text_options: { pinyin: false, stripSymbols: true, unifySymbols: true, stripBreaks: true } }),
    { pinyin: false, stripSymbols: true, unifySymbols: true, stripBreaks: true },
  );
});

test('pinyin hints are skipped for punctuation and whitespace', () => {
  assert.equal(isPinyinChar('你'), true);
  assert.equal(isPinyinChar('a'), true);
  assert.equal(isPinyinChar('，'), false);
  assert.equal(isPinyinChar('\n'), false);
  assert.equal(isPinyinChar(' '), false);
});
