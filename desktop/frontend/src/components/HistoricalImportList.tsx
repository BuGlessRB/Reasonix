import { useCallback, useEffect, useRef, useState } from "react";
import { app } from "../lib/bridge";
import { asArray } from "../lib/array";
import { useT } from "../lib/i18n";
import type { HistoricalImportStatus, HistoricalSessionView } from "../generated/desktopContract.generated";
import type { SessionRef } from "../lib/sessionRef";

export function HistoricalImportList({ active, onOpenSession }: { active: boolean; onOpenSession: (ref: SessionRef) => Promise<void> }) {
  const t = useT();
  const [status, setStatus] = useState<HistoricalImportStatus>({ items: [], running: false, paused: false, remaining: 0 });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [query, setQuery] = useState("");
  const generation = useRef(0);
  const requests = useRef(0);
  const reload = useCallback(async (scan = false) => {
    const current = generation.current;
    const request = ++requests.current;
    try {
      const next = await (scan ? app.ListHistoricalSessions() : app.GetHistoricalImportStatus());
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
      const ref = item.session ?? (await app.ImportHistoricalSession(item.id)).session;
      if (current === generation.current) await onOpenSession(ref);
    } catch (err) { if (current === generation.current) setError(String(err)); }
    finally { if (current === generation.current) setBusy(""); await reload(); }
  };
  const control = async (action: string) => {
    const current = generation.current;
    ++requests.current;
    try {
      setError("");
      const next = action === "start" ? await app.StartHistoricalImport([]) : await app.ControlHistoricalImport(action);
      if (current === generation.current) setStatus(next);
    } catch (err) { if (current === generation.current) setError(String(err)); }
  };
  return <section className="archived-sessions" aria-label={t("history.importTitle")}>
    <h3>{t("history.importTitle")}</h3>
    <p>{t("history.importDescription")}</p>
    <input aria-label={t("history.searchPlaceholder")} placeholder={t("history.searchPlaceholder")} value={query} onChange={event => setQuery(event.target.value)} />
    <div>
      <button className="btn btn--small" disabled={status.running || !!busy} onClick={() => void control("start")}>{t("history.importAll")}</button>
      {status.running && <>
        <button className="btn btn--small" onClick={() => void control(status.paused ? "resume" : "pause")}>{t(status.paused ? "history.importResume" : "history.importPause")}</button>
        <button className="btn btn--small" onClick={() => void control("cancel")}>{t("history.importCancel")}</button>
        <span role="status">{t("history.importRemaining")} {status.remaining}</span>
      </>}
      <button className="btn btn--small" onClick={() => void reload(true)}>{t("common.retry")}</button>
    </div>
    {error && <p role="alert">{error}</p>}
    {status.items.filter(item => item.title.toLowerCase().includes(query.toLowerCase())).map(item =>
      <div className="archived-sessions__row" key={item.id}>
        <span>{item.title}</span><small>{item.format}</small>
        <span>{t(`history.importStatus.${item.status}` as Parameters<typeof t>[0])}</span>
        {item.errorCode && <small>{item.errorCode === "source_busy" ? t("history.importSourceBusy") : t("history.importFailed")}</small>}
        <button className="btn btn--small" disabled={!!busy || item.status === "importing" || item.status === "archived" || item.status === "deleted"}
          onClick={() => void open(item)}>{t(item.session ? "history.openRestored" : "history.importOpen")}</button>
      </div>)}
    {status.items.length === 0 && <p>{t("history.noHistoricalSessions")}</p>}
  </section>;
}
