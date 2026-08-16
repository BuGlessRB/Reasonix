import { memo, useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { reason } from "../i18n/kernel";
import { t } from "../i18n";
import { createPortal } from "react-dom";
import type { AgentPort, ApprovalVerdict, Checkpoint, ContextBreakdown, JobEntry, McpEntry, RewindScope, SessionStatus, WorkspaceChanges } from "../port/port";
import type { RuntimeView } from "../port/hub";
import { fromHistory, initialState, quoteAmount, reduce } from "../state/session";
import { pairCheckpoints } from "../state/checkpoints";
import { initialTraj, reduceTraj } from "../state/trajectory";
import { Transcript } from "./Transcript";
import { Trajectory } from "./Trajectory";
import { Composer } from "./Composer";
import { RunStrip } from "./RunStrip";
import { SlottedView } from "./SlottedView";
import { key as slotKey, placement } from "./slots";
import { Metrics } from "./Metrics";
import { arrowTabs } from "./tablist";
import { tokensPerSecond } from "../port/tokens";

// PaneReport is what the window's own chrome needs from whichever pane has
// focus: everything else about a session stays inside the pane that owns it.
export interface PaneReport {
  status: SessionStatus | null;
  title: string;
  steer: number;
  run: string;
  // Whether the turn is actually moving or waiting on you. "halt" cannot answer
  // this: a reopened history sits at halt too, and closing that costs nothing.
  live: boolean;
  cost: string;
}

// A shared constant, not `?? []`: a fresh empty array every render reads as a
// changed prop to the rail below it.
const NO_JOBS: JobEntry[] = [];

interface Props {
  port: AgentPort;
  rt: RuntimeView;
  title: string;
  active: boolean;
  // Where the metrics rail lives. Only the focused pane renders into it, so the
  // column stays on the window's edge instead of appearing between two panes.
  sideHost: HTMLElement | null;
  side: boolean;
  onFocus: () => void;
  // Every pane reports, not just the focused one: a tab has to show that the
  // conversation behind it is still working.
  onReport: (id: string, report: PaneReport) => void;
  // Off-screen panes stay mounted — their stream, transcript and scroll
  // position are exactly what a tab switch must not throw away.
  visible: boolean;
  onSessionChanged: () => void;
  // Bumped when something outside this pane changed a setting that belongs to
  // its session. /status is polled only while a turn runs, so without this the
  // pane keeps reporting the posture it had when it opened.
  pulse: number;
  onFoldSide: () => void;
  onSettings: () => void;
}

function PaneView({ port, rt, title, active, visible, sideHost, side, onFocus, onReport, onSessionChanged, pulse, onFoldSide, onSettings }: Props) {
  const [s, dispatch] = useReducer(reduce, initialState);
  const [traj, trajDispatch] = useReducer(reduceTraj, initialTraj);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [tab, setTab] = useState<"flow" | "traj">("flow");
  const [pinned, setPinned] = useState(true);
  const [jump, setJump] = useState(0);
  const [mcp, setMcp] = useState<McpEntry[]>([]);
  const [elapsed, setElapsed] = useState(0);
  const [tps, setTps] = useState(0);
  const [tree, setTree] = useState<WorkspaceChanges | null>(null);
  const [ctx, setCtx] = useState<ContextBreakdown | null>(null);
  const [checkpoints, setCheckpoints] = useState<Checkpoint[]>([]);
  const [slots, setSlots] = useState<Record<string, string>>({});
  const flow = useRef<HTMLDivElement>(null);
  const startedAt = useRef(0);
  // Read by the 250ms tick without making it a dependency, so a delta arriving
  // between ticks does not restart the interval.
  const win = useRef(s.outWindow);
  win.current = s.outWindow;

  const reloadMcp = useCallback(() => {
    void port.mcp().then(setMcp).catch(() => setMcp([]));
  }, [port]);

  useEffect(
    () =>
      port.subscribe((ev) => {
        dispatch(ev);
        trajDispatch(ev);
        // A server finishing its handshake changes what /mcp answers, and this
        // is the only precise signal for it — the turn boundary below is the
        // fallback for changes that arrive without an event.
        if (ev.kind === "mcp_surface_ready" || ev.kind === "extension_status") reloadMcp();
      }),
    [port, reloadMcp],
  );

  useEffect(() => {
    let alive = true;
    port.trajectory().then((evs) => alive && evs.forEach((e) => trajDispatch(e))).catch(() => {});
    port.checkpoints().then((cps) => alive && setCheckpoints(cps)).catch(() => {});
    // The record and the numbers over it are two reads, not one. /status can go
    // to the network — the provider's wallet endpoint rides it — and pairing the
    // two made the conversation wait on a round trip that has nothing to do with
    // it. Whichever lands first shows what it knows.
    port.history().then((msgs) => {
      if (!alive) return;
      const restored = fromHistory(msgs);
      dispatch({ kind: "__restore", items: restored.items, plan: restored.plan } as never);
    });
    port.status().then((st) => {
      if (!alive) return;
      setStatus(st);
      dispatch({ kind: "__totals", hit: st.cacheHit, miss: st.cacheMiss, cost: quoteAmount(st.sessionCostQuote) } as never);
    });
    return () => {
      alive = false;
    };
  }, [port]);

  // /status is polled four times a second while a turn runs, and most of those
  // answers are word-for-word the previous one. Swapping in an equal object
  // would repaint the rail and the composer for no news at all.
  const applyStatus = useCallback((next: SessionStatus) => {
    setStatus((prev) => (prev && JSON.stringify(prev) === JSON.stringify(next) ? prev : next));
  }, []);

  const refreshStatus = useCallback(() => {
    port.status().then(applyStatus).catch(() => {});
  }, [port, applyStatus]);

  useEffect(() => {
    if (pulse) refreshStatus();
  }, [pulse, refreshStatus]);

  const fail = useCallback((e: unknown) => {
    // A refusal carries a code; say() turns it into this window's language.
    // Anything else is an ordinary failure and prints as itself.
    dispatch({ kind: "__error", text: reason(e) } as never);
  }, []);

  // Everything on screen belongs to one session; when the kernel moves this
  // pane to another one — a switch, a new session, a rewind — all of it has to
  // be re-read rather than patched.
  const reloadSession = useCallback(() => {
    trajDispatch({ kind: "__clear" } as never);
    port.trajectory().then((evs) => evs.forEach((e) => trajDispatch(e))).catch(() => {});
    port.checkpoints().then(setCheckpoints).catch(() => setCheckpoints([]));
    // Two reads, the same way the first mount takes them: the record does not
    // wait behind the numbers over it.
    port.history().then((msgs) => {
      const r = fromHistory(msgs);
      dispatch({ kind: "__restore", items: r.items, plan: r.plan } as never);
    });
    port.status().then((st) => {
      applyStatus(st);
      dispatch({ kind: "__totals", hit: st.cacheHit, miss: st.cacheMiss, cost: quoteAmount(st.sessionCostQuote) } as never);
    });
    onSessionChanged();
  }, [port, applyStatus, onSessionChanged]);

  // A rewind rewrites the transcript and the files under it, so the whole
  // session is re-read rather than patched — the same treatment a session
  // switch gets, for the same reason.
  const onPrepareRewind = useCallback((turn: number, scope: RewindScope) => port.prepareRewind(turn, scope), [port]);
  const onCommitRewind = useCallback(
    async (planId: string) => {
      const result = await port.commitRewind(planId);
      reloadSession();
      return result;
    },
    [port, reloadSession],
  );
  const onUndoRewind = useCallback((transactionId: string) => port.undoRewind(transactionId).then(reloadSession), [port, reloadSession]);

  // Both of these read only the user and tool cards, so they key off the
  // revision rather than the items array: a streamed answer leaves every card
  // they look at untouched, and recomputing them per chunk is the whole reason
  // a long session used to slow down. eslint would want `s.items` in the deps;
  // `s.revision` is the narrower truth. Same for the rail's two panels below.
  /* eslint-disable react-hooks/exhaustive-deps */
  const paired = useMemo(() => pairCheckpoints(s.items, checkpoints), [s.revision, checkpoints]);
  const counts = useMemo(() => {
    let steps = 0;
    let steer = 0;
    for (const i of s.items) {
      if (i.t === "tool") steps++;
      else if (i.t === "user" && i.pending) steer++;
    }
    return { steps, steer };
  }, [s.revision]);
  /* eslint-enable react-hooks/exhaustive-deps */

  // An MCP server connects lazily and fails at first use, so a turn boundary is
  // also when its status can have changed — no timer of its own needed.
  useEffect(() => {
    reloadMcp();
    // A finished turn is exactly when the kernel has one more checkpoint.
    port.checkpoints().then(setCheckpoints).catch(() => {});
    port.changes().then(setTree).catch(() => setTree(null));
  }, [reloadMcp, port, status?.sessionPath, s.running]);

  // One turn can be dozens of model round trips — the session this was measured
  // on ran thirty, from 9k tokens to 57k. Reading the gauge only at the turn
  // boundary froze it for the whole of that, which is exactly when someone
  // watches it. Usage arrives on every round trip, so it is the signal; the
  // kernel keys its own answer on the transcript version, so asking again
  // between trips costs nothing.
  // A fold replaces the history wholesale after its own usage has already been
  // counted, so round trips alone would leave the gauge showing the window from
  // before the compaction — the one moment it moves most.
  const folds = s.items.reduce((n, i) => n + (i.t === "compaction" && i.done ? 1 : 0), 0);
  const roundTrips = s.metrics.hit + s.metrics.miss;
  useEffect(() => {
    port.context().then(setCtx).catch(() => setCtx(null));
  }, [port, roundTrips, folds, status?.sessionPath, s.running]);

  // The sidebar has to hear about this pane's session twice: when the first
  // turn mints the file (before that there is no row to show) and when the turn
  // ends (that is when it has a generated title and a turn count). Without it a
  // brand-new conversation only appeared in the tree once its pane was closed.
  useEffect(() => {
    onSessionChanged();
  }, [status?.sessionPath, s.running, onSessionChanged]);

  // /status is the only source for background jobs and for settings the run does
  // not echo, so a live turn has to re-read it rather than infer from events.
  useEffect(() => {
    if (!s.running) {
      startedAt.current = 0;
      setTps(0);
      return;
    }
    if (!startedAt.current) startedAt.current = Date.now();
    const t = setInterval(() => {
      setElapsed((Date.now() - startedAt.current) / 1000);
      setTps(tokensPerSecond(win.current, Date.now()));
      refreshStatus();
    }, 250);
    return () => clearInterval(t);
  }, [s.running, refreshStatus]);

  useEffect(() => {
    void port.surfaceSlots().then(setSlots).catch(() => setSlots({}));
  }, [port]);

  // Where the user put each extension surface. Updated in place: a move is the
  // user's own action, so waiting for a round trip would read as a dead click.
  const moveSurface = useCallback(
    async (ext: { pluginId: string; surfaceId: string }, slot: string) => {
      const id = `${ext.pluginId}:${ext.surfaceId}`;
      setSlots((prev) => {
        const next = { ...prev };
        if (slot) next[id] = slot;
        else delete next[id];
        return next;
      });
      await port.assignSurface(id, slot).catch(fail);
    },
    [port, fail],
  );
  // Split once per change rather than per frame: a new array each render is a
  // changed prop, and that alone would keep the rail re-rendering all turn.
  const [atComposer, inRail] = useMemo(() => {
    const at: typeof s.views = [];
    const rail: typeof s.views = [];
    for (const v of s.views) (placement(v, slots) === "composer-trailing" ? at : rail).push(v);
    return [at, rail];
  }, [s.views, slots]);

  const submit = useCallback(
    (text: string) => {
      const steering = s.running;
      dispatch({ kind: "__user", text, pending: steering } as never);
      trajDispatch({ kind: "__user", text });
      if (steering) {
        port.steer(text).catch(fail);
        return;
      }
      port.submit(text).then(refreshStatus).catch(fail);
    },
    [port, s.running, refreshStatus, fail],
  );

  // Transcript rows are memoised on their item; a callback rebuilt every render
  // would defeat that on the two cards that take one.
  const onApprove = useCallback(
    (itemId: string, id: string, v: ApprovalVerdict) => {
      dispatch({ kind: "__decided", id: itemId, verdict: v } as never);
      port.approve(id, v).then(refreshStatus).catch(fail);
    },
    [port, refreshStatus, fail],
  );

  // Taking it back is only cheap while the conversation that caused it is still
  // on screen; the card stays, marked, so the record of what happened survives.
  const onForget = useCallback(
    (itemId: string, name: string) => {
      port.forgetMemory(name).then(() => dispatch({ kind: "__forgot", id: itemId } as never)).catch(fail);
    },
    [port, fail],
  );

  // An extension action reports back in its own words. Surfacing the result as
  // a notice keeps it in the transcript where the card that offered it sits.
  const onExtInvoke = useCallback(
    (name: string) => {
      port
        .invokeExtensionAction(name)
        .then((message) => {
          if (message.trim()) dispatch({ kind: "notice", level: "info", text: message });
        })
        .catch(fail);
    },
    [port, fail],
  );

  const onExtSubmit = useCallback(
    (pluginId: string, surfaceId: string, values: Record<string, unknown>) => {
      port.submitExtensionForm(pluginId, surfaceId, values).catch(fail);
    },
    [port, fail],
  );

  const onAnswer = useCallback(
    (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => {
      dispatch({ kind: "__decided", id: itemId, answers: answers.map((a) => a.selected) } as never);
      void port.answer(id, answers).catch(fail);
    },
    [port, fail],
  );

  // Where the bottom is moves as blocks mount under it, so this only asks the
  // transcript to follow again and lets it scroll itself into place.
  const toLatest = () => setJump((n) => n + 1);

  // A turn blocked on you is not a turn in motion: the glow says "it is moving"
  // and has to go out, and the action slot goes back to send so you can talk.
  const blocked = s.doing === "等你批准" || s.doing === "等你决定";
  const run = blocked ? "halt" : s.running ? "running" : s.items.length ? (s.doing === "已完成" ? "done" : "halt") : "idle";
  const cost = `${s.metrics.currency}${s.metrics.cost.toFixed(2)}`;

  // The chrome reads the focused pane. Reporting from an effect keeps it out of
  // render, where it would set state on the parent mid-paint.
  useEffect(() => {
    onReport(rt.id, { status, title, steer: counts.steer, run, live: s.running || blocked, cost });
  }, [rt.id, onReport, status, title, counts.steer, run, s.running, blocked, cost]);

  return (
    <section
      className="pane"
      data-run={run}
      data-off={visible ? undefined : ""}
      aria-hidden={visible ? undefined : true}
      data-active={active ? "" : undefined}
      aria-label={title}
      onMouseDownCapture={active ? undefined : onFocus}
      onFocusCapture={active ? undefined : onFocus}
    >
      <div className="tabs" role="tablist" onKeyDown={arrowTabs}>
        <button className="tab" role="tab" aria-selected={tab === "flow"} onClick={() => setTab("flow")}>
          {t("活动")}<span className="n">{s.items.length}</span>
        </button>
        <button className="tab" role="tab" aria-selected={tab === "traj"} onClick={() => setTab("traj")}>
          {t("轨迹")}<span className="n">{traj.rows.length}</span>
        </button>
      </div>

      <Transcript
        items={s.items}
        revision={s.revision}
        takeovers={s.takeovers}
        waiting={s.waiting}
        scroll={flow}
        hidden={tab !== "flow"}
        onPinned={setPinned}
        jump={jump}
        onSuggest={submit}
        onApprove={onApprove}
        onAnswer={onAnswer}
        onForget={onForget}
        onExtInvoke={onExtInvoke}
        onExtSubmit={onExtSubmit}
        checkpoints={paired}
        onPrepareRewind={onPrepareRewind}
        onCommitRewind={onCommitRewind}
        onUndoRewind={onUndoRewind}
      />

      {/* Mounted only while it is the tab on screen. Hiding it with an
          attribute left every row of it being rebuilt on each streamed
          delta — a second transcript's worth of work, drawn for nobody. */}
      <div className="scroll" data-pane="traj" hidden={tab !== "traj"}>
        {tab === "traj" && <Trajectory rows={traj.rows} onSave={(n, c) => port.saveText(n, c)} />}
      </div>

      <div className="compose">
        <button className="jump" hidden={pinned || tab !== "flow"} onClick={toLatest}>
          {t("↓ 回到最新")}
        </button>
        <span className="glowring" aria-hidden="true">
          <i />
        </span>
        <RunStrip doing={s.doing} metrics={s.metrics} steps={counts.steps} elapsed={elapsed} />
        {/* Views the user (or the extension) put next to the composer. They sit
            above it rather than inside it: the input box is the one thing an
            extension must never be able to crowd out. */}
        {atComposer.length > 0 && (
          <div className="slotrail">
            {atComposer.map((ext) => (
              <SlottedView
                key={slotKey(ext)}
                ext={ext}
                assigned={slots}
                onAction={(id) => void port.invokeExtensionAction(id).catch(fail)}
                onMove={(slot) => void moveSurface(ext, slot)}
              />
            ))}
          </div>
        )}
        <Composer port={port} status={status} running={s.running && !blocked} onSubmit={submit} onChanged={refreshStatus} onError={fail} />
        {s.error && (
          <div className="errbar" role="alert">
            <span>{s.error}</span>
            <button onClick={() => dispatch({ kind: "__error", text: "" } as never)}>{t("知道了")}</button>
          </div>
        )}
      </div>

      {active &&
        side &&
        sideHost &&
        createPortal(
          <Metrics
            metrics={s.metrics}
            plan={s.plan}
            items={s.items}
            revision={s.revision}
            jobs={status?.jobs ?? NO_JOBS}
            mcp={mcp}
            rate={tps}
            done={!s.running}
            tree={tree}
            ctx={ctx}
            yolo={status?.toolApprovalMode === "yolo"}
            onFold={onFoldSide}
            onSettings={onSettings}
            panels={s.panels}
            views={inRail}
            onMoveSurface={moveSurface}
            onExtInvoke={onExtInvoke}
          />,
          sideHost,
        )}
    </section>
  );
}

// Panes run at the same time: a frame arriving in one must not re-render the
// others, or two live conversations cost twice what one does.
export const Pane = memo(PaneView);
