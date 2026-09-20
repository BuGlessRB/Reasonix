import { useCallback, useEffect, useRef, useState } from "react";
import { app } from "../lib/bridge";
import { asArray } from "../lib/array";
import { useT } from "../lib/i18n";
import type { HistoricalImportStatus, HistoricalSessionView, HistoricalSourceUpdateView } from "../generated/desktopContract.generated";
import type { SessionRef } from "../lib/sessionRef";
import { desktopHost } from "../lib/desktopHost";

const emptyHistoricalStatus: HistoricalImportStatus = { items: [], running: false, paused: false, remaining: 0, completed: 0, blocked: 0, failed: 0 };

export function HistoricalImportList({ active, onOpenSession }: { active: boolean; onOpenSession?: (ref: SessionRef) => Promise<void> }) {
  const t = useT();
  const [status, setStatus] = useState<HistoricalImportStatus>(emptyHistoricalStatus);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const [updates, setUpdates] = useState<Record<string, HistoricalSourceUpdateView>>({});
  const generation = useRef(0);
  const requests = useRef(0);
  const reload = useCallback(async (scan = false) => {
    const current = generation.current;
    const request = ++requests.current;
    if (desktopHost().kind === "none") { setStatus(emptyHistoricalStatus); return; }
    try {
      const next = await (scan ? app.ListHistoricalSessions?.() : app.GetHistoricalImportStatus?.()) ?? emptyHistoricalStatus;
      if (current === generation.current && request === requests.current) setStatus({ ...next, items: asArray<HistoricalSessionView>(next.items) });
    } catch (err) { if (current === generation.current) setError(String(err)); }
  }, []);
  useEffect(() => {
    if (!active) return;
    setBusy("");
    void reload(true);
    const timer = setInterval(() => void reload(), 2000);
    return () => { generation.current++; clearInterval(timer); };
  }, [active, reload]);
  const open = async (item: HistoricalSessionView) => {
    if (busy) return;
    const current = generation.current;
    setBusy(item.id); setError("");
    try {
      const ref = item.session ?? (await app.ImportHistoricalSession!(item.id)).session;
      if (current === generation.current && onOpenSession) await onOpenSession(ref);
    } catch (err) { if (current === generation.current) setError(String(err)); }
    finally { if (current === generation.current) setBusy(""); await reload(); }
  };
  const control = async (action: string) => {
    const current = generation.current;
    ++requests.current;
    try {
      setError("");
      const next = action === "start" ? await app.StartHistoricalImport!([]) : await app.ControlHistoricalImport!(action);
      if (current === generation.current) setStatus(next);
    } catch (err) { if (current === generation.current) setError(String(err)); }
  };
  const checkUpdate = async (item: HistoricalSessionView) => {
    if (!item.source) return;
    setBusy(item.id); setError("");
    try {
      let update = await app.CheckHistoricalSourceUpdate!({ source: item.source });
      while (update.status === "checking") {
        await new Promise(resolve => setTimeout(resolve, 300));
        update = await app.CheckHistoricalSourceUpdate!({ source: item.source });
      }
      setUpdates(current => ({ ...current, [item.id]: update }));
    } catch (err) { setError(String(err)); }
    finally { setBusy(""); }
  };
  const prepareUpdate = async (item: HistoricalSessionView, update: HistoricalSourceUpdateView) => {
    if (!item.source || !update.version) return;
    setBusy(item.id); setError("");
    try { await app.PrepareHistoricalSourceVersion!(item.source, update.version); await reload(); }
    catch (err) { setError(String(err)); }
    finally { setBusy(""); }
  };
  return <section className="archived-sessions" aria-label={t("history.importTitle")}>
    <h3>{t("history.importTitle")}</h3>
    <p>{t("history.importDescription")}</p>
    <input aria-label={t("history.searchPlaceholder")} placeholder={t("history.searchPlaceholder")} value={query} onChange={event => setQuery(event.target.value)} />
    <div>
      <button className="btn btn--small" disabled={status.running || !!busy} onClick={() => void control("start")}>{t("history.importAll")}</button>
      {(status.running || (status.paused && status.remaining > 0)) && <>
        <button className="btn btn--small" onClick={() => void control(status.paused ? "resume" : "pause")}>{t(status.paused ? "history.importResume" : "history.importPause")}</button>
        <button className="btn btn--small" onClick={() => void control("cancel")}>{t("history.importCancel")}</button>
        <span role="status">{t("history.importRemaining")} {status.remaining} · {t("history.importStatus.imported")} {status.completed} · {t("history.importStatus.blocked")} {status.blocked} · {t("history.importStatus.failed")} {status.failed}</span>
      </>}
      <button className="btn btn--small" onClick={() => void reload(true)}>{t("common.retry")}</button>
    </div>
    {error && <p role="alert">{error}</p>}
    {status.items.filter(item => item.title.toLowerCase().includes(query.toLowerCase())).map(item =>
      <div className="archived-sessions__row" key={item.id}>
        <span>{item.title}</span><small>{item.format}</small>
        <span>{t(`history.importStatus.${item.status}` as Parameters<typeof t>[0])}</span>
        {item.errorCode && <small>{item.errorCode === "source_busy" ? t("history.importSourceBusy") : t("history.importFailed")}</small>}
        <button className="btn btn--small" disabled={!!busy || item.status === "importing" || item.status === "queued" || item.status === "archived" || item.status === "deleted"}
          onClick={() => void open(item)}>{t(item.session && onOpenSession ? "history.openRestored" : "history.importOpen")}</button>
        {item.session && item.source && <button className="btn btn--small" disabled={!!busy} onClick={() => void checkUpdate(item)}>{t("common.retry")} · {t("history.historicalSection")}</button>}
        {updates[item.id]?.status === "available" && <button className="btn btn--small" disabled={!!busy} onClick={() => void prepareUpdate(item, updates[item.id])}>{t("history.importOpen")} · {t("history.branchBadge")}</button>}
        {updates[item.id]?.status === "unchanged" && <small>{t("history.importStatus.imported")}</small>}
      </div>)}
    {status.items.length === 0 && <p>{t("history.noHistoricalSessions")}</p>}
  </section>;
}
