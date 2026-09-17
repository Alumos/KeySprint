// Sparse article offsets preserve paragraph breaks and existing saved sessions.
export type BufferMap = Record<number, string>;

export function nextPosition(chars: readonly string[], pos: number): number {
  while (pos < chars.length && chars[pos] === "\n") pos++;
  return pos;
}

export function previousPosition(chars: readonly string[], pos: number): number {
  while (pos >= 0 && chars[pos] === "\n") pos--;
  return pos;
}

export function insertText(chars: readonly string[], old: BufferMap, cursor: number, text: string) {
  const buffer = { ...old };
  let pos = cursor;
  for (const char of text) {
    pos = nextPosition(chars, pos);
    if (pos >= chars.length) break;
    if (buffer[pos] !== undefined) {
      for (let to = previousPosition(chars, chars.length - 1); to > pos;) {
        const from = previousPosition(chars, to - 1);
        if (buffer[from] === undefined) delete buffer[to];
        else buffer[to] = buffer[from];
        to = from;
      }
    }
    buffer[pos] = char;
    pos = nextPosition(chars, pos + 1);
  }
  return { buffer, cursor: pos };
}

export function deleteBefore(chars: readonly string[], old: BufferMap, cursor: number) {
  const pos = previousPosition(chars, cursor - 1);
  if (pos < 0) return { buffer: old, cursor };
  const buffer = { ...old };
  if (buffer[cursor] !== undefined) {
    let to = pos;
    for (let from = nextPosition(chars, to + 1); from < chars.length; from = nextPosition(chars, from + 1)) {
      if (buffer[from] === undefined) delete buffer[to];
      else buffer[to] = buffer[from];
      to = from;
    }
    delete buffer[to];
  } else delete buffer[pos];
  return { buffer, cursor: pos };
}

export function countInput(chars: readonly string[], buffer: BufferMap) {
  let typedCount = 0;
  let correct = 0;
  for (const index in buffer) {
    const value = buffer[index];
    if (value === undefined) continue;
    typedCount++;
    if (chars[Number(index)] === value) correct++;
  }
  return { typedCount, correct };
}

// Leave room beneath the active row for IME candidates; scroll only at an edge.
export function followScrollDelta(top: number, bottom: number, viewportTop: number, viewportBottom: number) {
  const height = viewportBottom - viewportTop;
  if (height <= 0) return 0;
  const margin = Math.min(96, height * 0.2);
  if (top >= viewportTop + 12 && bottom <= viewportBottom - margin) return 0;
  return top - (viewportTop + Math.max(12, (height - (bottom - top)) * 0.35));
}
