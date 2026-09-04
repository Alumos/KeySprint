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
import type { Article, Competition } from "../lib/types";

type BufferMap = Record<number, string>;
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
  onSelect,
}: CharCellProps) {
  const state = value !== undefined ? (value === char ? "is-correct" : "is-wrong") : "";
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

const punctuation =
  /[，。！？；：“”‘’、（）《》—…\s,.\-!?;:'"()\[\]{}<>/@#$%^&*+=~`|]/;
const parseTime = (s?: string) =>
  s ? new Date(s.includes("T") ? s : s.replace(" ", "T") + "Z").getTime() : 0;
const fmt = (n: number) =>
  `${String(Math.floor(n / 60)).padStart(2, "0")}:${String(n % 60).padStart(2, "0")}`;

export default function Compete() {
  const nav = useNavigate();
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
    const current = content?.querySelector<HTMLElement>(
      `.char-cell[data-char-index="${cursorRef.current}"]`,
    );
    const cells = content?.querySelectorAll<HTMLElement>(".char-cell");
    const fallback = cells?.length ? cells[cells.length - 1] : undefined;
    const anchor = current || fallback || content || panel;
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
  const chars = useMemo(() => Array.from(article?.content || ""), [article]);
  const typeable = useMemo(
    () => chars.map((ch, i) => (ch === "\n" ? -1 : i)).filter((x) => x >= 0),
    [chars],
  );
  const typedCount = Object.keys(buffer).length;
  const correct = Object.entries(buffer).filter(
    ([i, ch]) => chars[Number(i)] === ch,
  ).length;
  const accuracy = typedCount ? correct / typedCount : 1;
  const speed = elapsed ? correct / (elapsed / 60) : 0;
  const progress = typeable.length ? (typedCount / typeable.length) * 100 : 0;
  const key = comp ? `keysprint_session_${comp.id}_${studentId}` : "";
  const py = useMemo(
    () =>
      chars.map((ch) =>
        ch === "\n" || punctuation.test(ch)
          ? ""
          : pinyin(ch, { toneType: "symbol" }),
      ),
    [chars],
  );
  const next = useCallback(
    (pos: number) => {
      while (pos < chars.length && chars[pos] === "\n") pos++;
      return pos;
    },
    [chars],
  );
  const prev = useCallback(
    (pos: number) => {
      while (pos >= 0 && chars[pos] === "\n") pos--;
      return pos;
    },
    [chars],
  );
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
      localStorage.setItem(key, JSON.stringify(current));
    },
    [key, buffer, cursor, anchor, late],
  );
  const loadArticle = useCallback(
    async (c: Competition, isLate: boolean) => {
      const a = await api<Article>(`/api/articles/${c.article_id}`);
      setArticle(a);
      setComp(c);
      const storageKey = `keysprint_session_${c.id}_${studentId}`;
      const saved = JSON.parse(
        localStorage.getItem(storageKey) || "null",
      ) as Session | null;
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
    persist({ submitted: true });
    if (late || typedCount === 0) {
      setSubmitState("idle");
      return;
    }
    const payload = {
      competitionId: comp!.id,
      studentId,
      studentName,
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
    comp,
    correct,
    late,
    persist,
    studentId,
    studentName,
    typedCount,
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
    function connect() {
      const ws = new WebSocket(wsURL());
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
        if (!stopped) setTimeout(connect, 1500);
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
      socket.current?.close();
    };
  }, [comp?.id, loadArticle, studentId, studentName]);
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
        if (raw >= comp.time_limit) finish();
      }
    };
    tick();
    const t = setInterval(tick, 250);
    return () => clearInterval(t);
  }, [anchor, comp, finish, late, status]);
  useEffect(() => {
    if (status === "running") persist();
  }, [buffer, cursor, persist, status]);
  function insert(text: string) {
    if (status !== "running") return;
    setBuffer((old) => {
      const b = { ...old };
      let pos = cursor;
      for (const ch of Array.from(text)) {
        pos = next(pos);
        if (pos >= chars.length) break;
        const rest = typeable.filter((i) => i >= pos);
        if (b[pos] !== undefined) {
          for (let i = rest.length - 1; i > 0; i--) b[rest[i]] = b[rest[i - 1]];
        }
        b[pos] = ch;
        pos = next(pos + 1);
      }
      bufferRef.current = b;
      setCursor(pos);
      if (typeable.every((i) => b[i] !== undefined)) setTimeout(finish, 0);
      return b;
    });
  }
  function backspace() {
    if (status !== "running") return;
    const pos = prev(cursor - 1);
    if (pos < 0) return;
    setBuffer((old) => {
      const b = { ...old };
      const rest = typeable.filter((i) => i >= pos);
      if (b[cursor] !== undefined) {
        for (let i = 0; i < rest.length - 1; i++) b[rest[i]] = b[rest[i + 1]];
        delete b[rest[rest.length - 1]];
      } else delete b[pos];
      bufferRef.current = b;
      return b;
    });
    setCursor(pos);
  }
  function furthest() {
    const used = Object.keys(bufferRef.current).map(Number);
    return next(used.length ? Math.max(...used) + 1 : 0);
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
          <strong>KeySprint</strong>
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
              detail: `范文共 ${typeable.length} 字`,
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
              className={`typing-content ${comp?.pinyin_enabled === false ? "without-pinyin" : ""}`}
            >
              {chars.map((ch, i) =>
                ch === "\n" ? (
                  <br key={i} />
                ) : (
                  <CharCell
                    key={i}
                    index={i}
                    char={ch}
                    pinyinText={comp?.pinyin_enabled === false ? "" : py[i]}
                    value={buffer[i]}
                    caret={cursor === i && focused && status === "running"}
                    onSelect={selectChar}
                  />
                ),
              )}
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
