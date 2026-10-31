import {
  ArrowLeft,
  CheckCircle2,
  Clock3,
  Keyboard,
  RotateCcw,
  Send,
  Wifi,
  WifiOff,
} from "lucide-react";
import { pinyin } from "pinyin-pro";
import {
  FormEvent,
  useCallback,
  useEffect,
  memo,
  useMemo,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useNavigate } from "react-router-dom";
import { Badge, Button, Modal, StatStrip } from "../components/ui";
import { api, json, wsURL } from "../lib/api";
import { useSite } from "../lib/site";
import type { Article, Competition } from "../lib/types";
import {
  applyTextOptions,
  equivalentChar,
  isPinyinChar,
  resolveTextOptions,
} from "../lib/textOptions";
import {
  countInput,
  deleteBefore,
  followScrollDelta,
  insertText,
  nextPosition,
  previousPosition,
  type BufferMap,
} from "../lib/typing";

type Session = {
  buffer: BufferMap;
  cursor: number;
  anchor: number;
  late: boolean;
  submitted: boolean;
};

type CharCellProps = {
  index: number;
  char: string;
  pinyinText: string;
  value?: string;
  caret: boolean;
  match: (expected: string, typed: string) => boolean;
  onSelect: (index: number) => void;
};

// A competition can contain hundreds of characters. Keeping each cell
// memoized means a keystroke updates only the edited cell and the caret cells,
// instead of reconciling the entire article tree.
const CharCell = memo(function CharCell({
  index,
  char,
  pinyinText,
  value,
  caret,
  match,
  onSelect,
}: CharCellProps) {
  const state = value !== undefined ? (match(char, value) ? "is-correct" : "is-wrong") : "";
  return (
    <span
      className={`char-cell ${state} ${caret ? "is-caret" : ""}`}
      data-char-index={index}
      onClick={(event) => {
        event.stopPropagation();
        onSelect(index);
      }}
    >
      <small>{pinyinText}</small>
      <b>{char}</b>
      <i>{value || " "}</i>
    </span>
  );
});

const parseTime = (s?: string) =>
  s ? new Date(s.includes("T") ? s : s.replace(" ", "T") + "Z").getTime() : 0;
const fmt = (n: number) =>
  `${String(Math.floor(n / 60)).padStart(2, "0")}:${String(n % 60).padStart(2, "0")}`;

export default function Compete() {
  const nav = useNavigate();
  const { site } = useSite();
  const studentId = sessionStorage.getItem("studentId") || "";
  const studentName = sessionStorage.getItem("studentName") || "";
  const code = sessionStorage.getItem("competeCode") || "";
  const [comp, setComp] = useState<Competition | null>(null);
  const [article, setArticle] = useState<Article | null>(null);
  const [status, setStatus] = useState<
    "loading" | "waiting" | "running" | "finished" | "error"
  >("loading");
  const [buffer, setBuffer] = useState<BufferMap>({});
  const bufferRef = useRef<BufferMap>({});
  const [cursor, setCursor] = useState(0);
  const [focused, setFocused] = useState(false);
  const [anchor, setAnchor] = useState(0);
  const [elapsed, setElapsed] = useState(0);
  const [remaining, setRemaining] = useState(0);
  const [late, setLate] = useState(false);
  const [resultOpen, setResultOpen] = useState(false);
  const [submitState, setSubmitState] = useState<
    "idle" | "submitting" | "ok" | "error"
  >("idle");
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const typingPanel = useRef<HTMLElement>(null);
  const typingContent = useRef<HTMLDivElement>(null);
  const cursorRef = useRef(cursor);
  cursorRef.current = cursor;
  const positionImeInput = useCallback(() => {
    const element = input.current;
    const panel = typingPanel.current;
    if (!element || !panel) return;

    const content = typingContent.current;
    const current = content?.children.item(cursorRef.current);
    const anchor = current || content?.lastElementChild || content || panel;
    const panelRect = panel.getBoundingClientRect();
    const anchorRect = anchor.getBoundingClientRect();

    // Keep the IME host in the typing panel so native candidate windows track
    // the current character while page scrolling remains under our control.
    element.style.left = `${Math.max(0, anchorRect.left - panelRect.left)}px`;
    element.style.top = `${Math.max(0, anchorRect.bottom - panelRect.top + 2)}px`;
  }, []);
  const focusInput = useCallback(() => {
    const element = input.current;
    if (!element) return;
    positionImeInput();
    const x = window.scrollX;
    const y = window.scrollY;
    element.focus({ preventScroll: true });
    // Keep older browsers from moving the race viewport while the IME opens.
    if (window.scrollX !== x || window.scrollY !== y) window.scrollTo(x, y);
    positionImeInput();
  }, [positionImeInput]);
  const socket = useRef<WebSocket | null>(null);
  const submitted = useRef(false);
  const composing = useRef(false);
  const finishRef = useRef<() => void>(() => {});
  const statusRef = useRef(status);
  statusRef.current = status;
  useLayoutEffect(() => {
    if (status === "running" && focused) {
      const content = typingContent.current;
      const active = content?.children.item(cursor) || content?.lastElementChild;
      if (active) {
        const rect = active.getBoundingClientRect();
        const viewport = window.visualViewport;
        const top = viewport?.offsetTop || 0;
        const bottom = top + (viewport?.height || window.innerHeight);
        const headerBottom = document.querySelector(".race-topbar")?.getBoundingClientRect().bottom || 0;
        const delta = followScrollDelta(rect.top, rect.bottom, Math.max(top, headerBottom), bottom);
        if (delta) window.scrollBy({ top: delta, behavior: "instant" });
      }
    }
    positionImeInput();
  }, [article, cursor, focused, positionImeInput, status]);
  useEffect(() => {
    let frame = 0;
    const schedulePosition = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(() => {
        frame = 0;
        positionImeInput();
      });
    };
    window.addEventListener("resize", schedulePosition);
    window.addEventListener("scroll", schedulePosition, true);
    const viewport = window.visualViewport;
    viewport?.addEventListener("resize", schedulePosition);
    viewport?.addEventListener("scroll", schedulePosition);
    return () => {
      window.removeEventListener("resize", schedulePosition);
      window.removeEventListener("scroll", schedulePosition, true);
      viewport?.removeEventListener("resize", schedulePosition);
      viewport?.removeEventListener("scroll", schedulePosition);
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, [positionImeInput]);
  const options = useMemo(() => resolveTextOptions(comp), [comp]);
  const content = useMemo(
    () => applyTextOptions(article?.content || "", options),
    [article, options],
  );
  const chars = useMemo(() => Array.from(content), [content]);
  const totalCount = useMemo(() => chars.reduce((n, ch) => n + Number(ch !== "\n"), 0), [chars]);
  const sameChar = useCallback(
    (expected: string, typed: string) => equivalentChar(expected, typed, options),
    [options],
  );
  const { typedCount, correct } = useMemo(
    () => countInput(chars, buffer, sameChar),
    [chars, buffer, sameChar],
  );
  const accuracy = typedCount ? correct / typedCount : 1;
  const speed = elapsed ? correct / (elapsed / 60) : 0;
  const progress = totalCount ? (typedCount / totalCount) * 100 : 0;
  const key = comp ? `keysprint_session_${comp.id}_${studentId}` : "";
  const py = useMemo(
    () =>
      options.pinyin
        ? chars.map((ch) => (isPinyinChar(ch) ? pinyin(ch, { toneType: "symbol" }) : ""))
        : [],
    [chars, options.pinyin],
  );
  const next = useCallback((pos: number) => nextPosition(chars, pos), [chars]);
  const prev = useCallback((pos: number) => previousPosition(chars, pos), [chars]);
  const persist = useCallback(
    (patch: Partial<Session> = {}) => {
      if (!key) return;
      const current: Session = {
        buffer,
        cursor,
        anchor,
        late,
        submitted: submitted.current,
        ...patch,
      };
      try {
        localStorage.setItem(key, JSON.stringify(current));
      } catch { /* Unavailable storage must not interrupt typing or submission. */ }
    },
    [key, buffer, cursor, anchor, late],
  );
  const loadArticle = useCallback(
    async (c: Competition, isLate: boolean) => {
      const a = await api<Article>(
        `/api/articles/${c.article_id}?code=${encodeURIComponent(c.access_code)}`,
      );
      setArticle(a);
      setComp(c);
      const storageKey = `keysprint_session_${c.id}_${studentId}`;
      let saved: Session | null = null;
      try {
        saved = JSON.parse(localStorage.getItem(storageKey) || "null") as Session | null;
      } catch { /* A corrupt or unavailable cache starts a fresh session. */ }
      if (saved) {
        const restored = saved.buffer || {};
        bufferRef.current = restored;
        setBuffer(restored);
        setCursor(saved.cursor || 0);
        setAnchor(saved.anchor || Date.now());
        setLate(saved.late);
        submitted.current = saved.submitted;
        if (saved.submitted) {
          setStatus("finished");
          setResultOpen(true);
        } else setStatus("running");
      } else {
        bufferRef.current = {};
        setBuffer({});
        setCursor(
          nextPosition(Array.from(applyTextOptions(a.content, resolveTextOptions(c))), 0),
        );
        submitted.current = false;
        setAnchor(Date.now());
        setLate(isLate);
        setStatus("running");
      }
      setTimeout(focusInput, 50);
    },
    [focusInput, studentId],
  );
  const finish = useCallback(async () => {
    if (submitted.current) return;
    submitted.current = true;
    setStatus("finished");
    setResultOpen(true);
    persist({ submitted: true, buffer: bufferRef.current, cursor: cursorRef.current });
    const { typedCount, correct } = countInput(chars, bufferRef.current, sameChar);
    if (late || typedCount === 0) {
      setSubmitState("idle");
      return;
    }
    const payload = {
      competitionId: comp!.id,
      studentId,
      studentName,
      accessCode: code,
      classId: Number(sessionStorage.getItem("studentClassId") || 0) || undefined,
      correctCount: correct,
      typedCount,
      duration: Math.min(
        comp!.time_limit,
        Math.floor((Date.now() - anchor) / 1000),
      ),
    };
    setSubmitState("submitting");
    for (const delay of [0, 800, 1800, 3500]) {
      if (delay) await new Promise((r) => setTimeout(r, delay));
      try {
        await api("/api/results", json("POST", payload));
        setSubmitState("ok");
        return;
      } catch {}
    }
    setSubmitState("error");
  }, [
    anchor,
    chars,
    code,
    comp,
    late,
    persist,
    sameChar,
    studentId,
    studentName,
  ]);
  finishRef.current = () => {
    void finish();
  };
  useEffect(() => {
    if (!studentId || !code) {
      nav("/student", { replace: true });
      return;
    }
    let cancelled = false;
    async function boot() {
      try {
        const c = await api<Competition>(
          `/api/competition/by-code/${encodeURIComponent(code)}`,
        );
        if (cancelled) return;
        setComp(c);
        if (c.status === "running") await loadArticle(c, true);
        else setStatus("waiting");
      } catch (e) {
        setError((e as Error).message);
        setStatus("error");
      }
    }
    boot();
    return () => {
      cancelled = true;
    };
  }, [code, loadArticle, nav, studentId]);
  useEffect(() => {
    if (!comp) return;
    const competitionId = comp.id;
    let stopped = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    function connect() {
      if (stopped) return;
      const ws = new WebSocket(
        `${wsURL()}?code=${encodeURIComponent(code)}&competitionId=${competitionId}` +
          `&studentId=${encodeURIComponent(studentId)}&studentName=${encodeURIComponent(studentName)}`,
      );
      socket.current = ws;
      ws.onopen = () => {
        setConnected(true);
        ws.send(
          JSON.stringify({
            type: "student:join",
            competitionId,
            studentId,
            studentName,
          }),
        );
      };
      ws.onclose = () => {
        setConnected(false);
        if (!stopped) reconnectTimer = setTimeout(connect, 1500);
      };
      ws.onmessage = (e) => {
        const msg = JSON.parse(e.data);
        if (msg.type === "competition:start") loadArticle(msg.data, false);
        if (msg.type === "competition:stop" && statusRef.current === "running")
          finishRef.current();
      };
    }
    connect();
    return () => {
      stopped = true;
      clearTimeout(reconnectTimer);
      socket.current?.close();
    };
  }, [code, comp?.id, loadArticle, studentId, studentName]);
  useEffect(() => {
    if (status !== "waiting" || !code) return;
    const t = setInterval(async () => {
      try {
        const c = await api<Competition>(
          `/api/competition/by-code/${encodeURIComponent(code)}`,
        );
        if (c.status === "running") loadArticle(c, false);
      } catch {}
    }, 2500);
    return () => clearInterval(t);
  }, [code, loadArticle, status]);
  useEffect(() => {
    if (status !== "running" || !comp || !anchor) return;
    const tick = () => {
      const raw = Math.max(0, Math.floor((Date.now() - anchor) / 1000));
      setElapsed(Math.min(comp.time_limit, raw));
      if (late) {
        const global = Math.max(
          0,
          comp.time_limit -
            Math.floor((Date.now() - parseTime(comp.started_at)) / 1000),
        );
        setRemaining(global);
      } else {
        setRemaining(Math.max(0, comp.time_limit - raw));
        if (raw >= comp.time_limit) finishRef.current();
      }
    };
    tick();
    const t = setInterval(tick, 250);
    return () => clearInterval(t);
  }, [anchor, comp, late, status]);
  // Persisting every keystroke rewrites the whole buffer as JSON. Debounce to
  // one write per idle moment and flush once more when the page goes away.
  const persistRef = useRef<() => void>(() => {});
  persistRef.current = persist;
  useEffect(() => {
    if (status !== "running") return;
    const timer = window.setTimeout(persist, 400);
    return () => window.clearTimeout(timer);
  }, [buffer, cursor, persist, status]);
  useEffect(() => {
    const flush = () => persistRef.current();
    window.addEventListener("pagehide", flush);
    return () => window.removeEventListener("pagehide", flush);
  }, []);
  // Finish after React commits the final input, so both the modal and payload
  // include the last character (also safe with StrictMode updater replay).
  useEffect(() => {
    if (status === "running" && totalCount > 0 && typedCount === totalCount) finishRef.current();
  }, [status, totalCount, typedCount]);
  function applyEdit(edit: { buffer: BufferMap; cursor: number }) {
    bufferRef.current = edit.buffer;
    cursorRef.current = edit.cursor;
    setBuffer(edit.buffer);
    setCursor(edit.cursor);
  }
  function insert(text: string) {
    if (status !== "running" || submitted.current) return;
    applyEdit(insertText(chars, bufferRef.current, cursorRef.current, text));
  }
  function backspace() {
    if (status !== "running" || submitted.current) return;
    applyEdit(deleteBefore(chars, bufferRef.current, cursorRef.current));
  }
  function furthest() {
    let last = -1;
    for (const index in bufferRef.current) last = Math.max(last, Number(index));
    return next(last + 1);
  }
  const selectChar = useCallback(
    (index: number) => {
      if (index <= furthest()) {
        // React state updates are async; update the positioning ref first so
        // the IME host follows the clicked cell on the same focus call.
        cursorRef.current = index;
        setCursor(index);
      }
      focusInput();
    },
    [focusInput, next],
  );
  const articleCells = useMemo(
    () => chars.map((ch, i) => ch === "\n" ? <br key={i} /> : (
      <CharCell
        key={i}
        index={i}
        char={ch}
        pinyinText={py[i] || ""}
        value={buffer[i]}
        caret={cursor === i && focused && status === "running"}
        match={sameChar}
        onSelect={selectChar}
      />
    )),
    [chars, py, buffer, cursor, focused, status, sameChar, selectChar],
  );
  function keydown(e: React.KeyboardEvent) {
    if (composing.current) return;
    if (e.key === "Backspace") {
      e.preventDefault();
      backspace();
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      setCursor(Math.max(0, prev(cursor - 1)));
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      setCursor(Math.min(furthest(), next(cursor + 1)));
    }
  }
  function inputEvent(e: FormEvent<HTMLInputElement>) {
    if (composing.current) return;
    const el = e.currentTarget;
    if (el.value) {
      insert(el.value);
      el.value = "";
    }
  }
  if (status === "loading")
    return (
      <main className="race-loading">
        <Keyboard size={28} />
        <strong>正在进入赛场</strong>
        <span>正在同步比赛状态和范文</span>
      </main>
    );
  if (status === "error")
    return (
      <main className="race-loading">
        <WifiOff size={28} />
        <strong>无法进入比赛</strong>
        <span>{error}</span>
        <Button variant="outline" onClick={() => nav("/student")}>
          返回入口
        </Button>
      </main>
    );
  return (
    <main className="race-shell">
      <header className="race-topbar">
        <div className="race-brand">
          <Keyboard size={18} />
          <strong>{site.title}</strong>
          <span>{comp?.article_title || "拼音打字竞赛"}</span>
        </div>
        <div className="race-user">
          <span>
            {studentId} · {studentName}
          </span>
          {connected ? <Wifi size={14} /> : <WifiOff size={14} />}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => nav("/student")}
            title="退出比赛"
          >
            <ArrowLeft size={16} />
          </Button>
        </div>
      </header>
      <div className="race-workspace">
        <StatStrip
          items={[
            {
              label: "剩余时间",
              value: fmt(remaining),
              detail: late ? "随全班结束" : "个人完整计时",
            },
            {
              label: "当前速度",
              value: speed.toFixed(1),
              detail: "正确字 / 分钟",
            },
            {
              label: "正确率",
              value: `${(accuracy * 100).toFixed(1)}%`,
              detail: `${correct} / ${typedCount}（正确 / 已输入）`,
            },
            {
              label: "完成进度",
              value: `${progress.toFixed(0)}%`,
              detail: `范文共 ${totalCount} 字`,
            },
          ]}
        />
        {late && (
          <div className="race-warning">
            你在比赛开始后进入，本次可继续练习，但成绩不会计入排行榜。
          </div>
        )}
        <section
          ref={typingPanel}
          className="typing-panel"
          onClick={focusInput}
        >
          <div className="typing-panel-head">
            <div>
              <Badge
                tone={
                  status === "running"
                    ? "success"
                    : status === "waiting"
                      ? "warning"
                      : "neutral"
                }
              >
                {status === "running"
                  ? "比赛进行中"
                  : status === "waiting"
                    ? "等待教师发令"
                    : "比赛已结束"}
              </Badge>
              <span>{article?.title || "等待加载范文"}</span>
            </div>
            <span>点击文字区域后使用中文输入法输入</span>
          </div>
          {status === "waiting" ? (
            <div className="waiting-state">
              <Clock3 size={30} />
              <strong>等待比赛开始</strong>
              <span>教师发令后，范文会自动出现</span>
            </div>
          ) : (
            <div
              ref={typingContent}
              className={`typing-content ${options.pinyin ? "" : "without-pinyin"}`}
            >
              {articleCells}
            </div>
          )}
          <input
            ref={input}
            className="hidden-ime"
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            onInput={inputEvent}
            onKeyDown={keydown}
            onCompositionStart={() => {
              composing.current = true;
            }}
            onCompositionEnd={(e) => {
              composing.current = false;
              insert(e.data);
              e.currentTarget.value = "";
            }}
          />
        </section>
      </div>
      <Modal
        open={resultOpen}
        onOpenChange={setResultOpen}
        title="本次输入结果"
        description={
          late
            ? "迟到练习成绩不计入教师排行榜"
            : submitState === "ok"
              ? "成绩已提交"
              : "正在保存成绩"
        }
      >
        <div className="result-summary">
          <CheckCircle2 size={28} />
          <strong>
            {speed.toFixed(1)} <small>字 / 分钟</small>
          </strong>
          <div>
            <span>
              正确字数 <b>{correct}</b>
            </span>
            <span>
              正确率 <b>{(accuracy * 100).toFixed(1)}%</b>
            </span>
            <span>
              用时 <b>{fmt(elapsed)}</b>
            </span>
          </div>
          {submitState === "error" && (
            <div className="notice notice-error">
              成绩提交失败，请保持页面打开并点击重试。
            </div>
          )}
          <div className="toolbar-row" style={{ justifyContent: "center" }}>
            {submitState === "error" && (
              <Button
                onClick={() => {
                  submitted.current = false;
                  finish();
                }}
              >
                <Send size={14} />
                重试提交
              </Button>
            )}
            <Button variant="outline" onClick={() => nav("/student")}>
              <RotateCcw size={14} />
              返回入口
            </Button>
          </div>
        </div>
      </Modal>
    </main>
  );
}
