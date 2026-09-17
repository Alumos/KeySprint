import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// Use the project's compiler so these tests also run on Node 22 without a TS loader.
const source = readFileSync(new URL('../src/lib/typing.ts', import.meta.url), 'utf8');
const { outputText } = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext } });
const { insertText, deleteBefore, countInput, followScrollDelta } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('composition commits multiple Unicode characters across paragraph breaks', () => {
  const chars = Array.from('\n你好\n\n🌟世界\n');
  const edit = insertText(chars, {}, 0, '你好🌟世界多余');
  assert.equal(edit.cursor, chars.length);
  assert.deepEqual(edit.buffer, { 1: '你', 2: '好', 5: '🌟', 6: '世', 7: '界' });
  assert.deepEqual(countInput(chars, edit.buffer), { typedCount: 5, correct: 5 });
});

test('insertion in the middle keeps sparse storage and counts only entered characters', () => {
  const chars = Array.from('甲乙\n丙丁戊己');
  const old = { 0: '甲', 1: '丙', 3: '丁' };
  const edit = insertText(chars, old, 1, '乙');
  assert.deepEqual(edit.buffer, { 0: '甲', 1: '乙', 3: '丙', 4: '丁' });
  assert.deepEqual(countInput(chars, edit.buffer), { typedCount: 4, correct: 4 });
  assert.deepEqual(old, { 0: '甲', 1: '丙', 3: '丁' });
  assert.deepEqual(JSON.parse(JSON.stringify(edit.buffer)), edit.buffer);
});

test('backspace shifts text across paragraphs and removes empty tail entries', () => {
  const chars = Array.from('甲乙\n丙丁戊');
  assert.deepEqual(deleteBefore(chars, { 0: '甲', 1: '错', 3: '乙', 4: '丙' }, 3), {
    buffer: { 0: '甲', 1: '乙', 3: '丙' }, cursor: 1,
  });
  assert.deepEqual(deleteBefore(chars, { 0: '甲', 1: '乙' }, 3), { buffer: { 0: '甲' }, cursor: 1 });
  assert.deepEqual(deleteBefore(chars, {}, 0), { buffer: {}, cursor: 0 });
});

test('completion includes the last character and accuracy retains wrong characters', () => {
  const chars = Array.from('你好');
  const edit = insertText(chars, { 0: '错' }, 1, '好');
  assert.deepEqual(countInput(chars, edit.buffer), { typedCount: 2, correct: 1 });
});

test('scrolling keeps visible rows still and follows both lower and upper edges', () => {
  assert.equal(followScrollDelta(200, 272, 52, 800), 0);
  assert.ok(followScrollDelta(690, 762, 52, 800) > 0);
  assert.ok(followScrollDelta(20, 92, 52, 800) < 0);
  assert.ok(followScrollDelta(300, 372, 52, 400) > 0);
  assert.equal(followScrollDelta(0, 72, 52, 52), 0);
});

test('long article insertion never creates phantom typed characters', () => {
  const chars = Array.from('甲'.repeat(10000));
  const edit = insertText(chars, { 0: '甲' }, 0, '乙');
  assert.deepEqual(countInput(chars, edit.buffer), { typedCount: 2, correct: 1 });
  assert.equal(Object.keys(edit.buffer).length, 2);
  assert.deepEqual(deleteBefore(chars, edit.buffer, 1).buffer, { 0: '甲' });
});
