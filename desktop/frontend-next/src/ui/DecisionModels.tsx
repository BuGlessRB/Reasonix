import { useEffect, useState } from "react";
import type { DecisionModels, DecisionModelsDraft } from "../port/decision";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import { Switch } from "./Switch";

type Port = { decisionModels(): Promise<DecisionModels>; saveDecisionModels(v: DecisionModelsDraft): Promise<void> };

export function DecisionModelsPanel({ port, onChanged }: { port: Port; onChanged: () => void }) {
  const [value, setValue] = useState<DecisionModels | null>(null);
  const [typeSafeKey, setTypeSafeKey] = useState("");
  const [layaKey, setLayaKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { port.decisionModels().then(setValue).catch((e) => setError(reason(e))); }, [port]);
  if (!value) return <p className="note">{error || t("正在读取…")}</p>;
  const updateTypeSafe = (patch: Partial<DecisionModels["typeSafe"]>) => setValue({ ...value, typeSafe: { ...value.typeSafe, ...patch } });
  const updateLaya = (patch: Partial<DecisionModels["laya"]>) => setValue({ ...value, laya: { ...value.laya, ...patch } });
  const save = async () => {
    setBusy(true); setError("");
    try {
      await port.saveDecisionModels({ ...value, typeSafe: { ...value.typeSafe, apiKey: typeSafeKey }, laya: { ...value.laya, httpApiKey: layaKey } });
      setTypeSafeKey(""); setLayaKey(""); onChanged();
      setValue(await port.decisionModels());
    } catch (e) { setError(reason(e)); } finally { setBusy(false); }
  };
  return <>
    <div className="lrow"><span className="tx"><span className="lb">TypeSafe AI</span><span className="ds">{t("托管的 System One 决策接口")}</span></span></div>
    <label className="lrow"><span className="tx"><span className="lb">{t("接口地址")}</span></span><input data-action="decision.typesafe-url" value={value.typeSafe.baseUrl} onChange={(e) => updateTypeSafe({ baseUrl: e.target.value })} placeholder="https://api.typesafe.ai" /></label>
    <label className="lrow"><span className="tx"><span className="lb">{t("模型")}</span></span><input data-action="decision.typesafe-model" value={value.typeSafe.model} onChange={(e) => updateTypeSafe({ model: e.target.value })} placeholder="system-one" /></label>
    <label className="lrow"><span className="tx"><span className="lb">API Key</span><span className="ds">{value.typeSafe.hasKey ? t("已保存；留空则保持不变") : t("保存在本机凭据存储")}</span></span><input data-action="decision.typesafe-key" type="password" value={typeSafeKey} onChange={(e) => setTypeSafeKey(e.target.value)} /></label>
    <div className="lrow"><span className="tx"><span className="lb">Laya</span><span className="ds">{t("可使用本地 Python 模型或兼容 HTTP 网关")}</span></span><Switch data-action="decision.laya-local" on={value.laya.local} busy={busy} label={t("启用 Laya 本地模型")} onClick={() => updateLaya({ local: !value.laya.local })} /></div>
    <label className="lrow"><span className="tx"><span className="lb">{t("Python 路径")}</span></span><input data-action="decision.laya-python" value={value.laya.python} onChange={(e) => updateLaya({ python: e.target.value })} placeholder="python" /></label>
    <label className="lrow"><span className="tx"><span className="lb">{t("Laya 模型")}</span></span><input data-action="decision.laya-model" value={value.laya.model} onChange={(e) => updateLaya({ model: e.target.value })} placeholder="auto" /></label>
    <label className="lrow"><span className="tx"><span className="lb">{t("HTTP 网关")}</span></span><input data-action="decision.laya-url" value={value.laya.httpBaseUrl} onChange={(e) => updateLaya({ httpBaseUrl: e.target.value })} placeholder="http://127.0.0.1:8000" /></label>
    <label className="lrow"><span className="tx"><span className="lb">{t("网关 API Key")}</span><span className="ds">{value.laya.hasHttpKey ? t("已保存；留空则保持不变") : t("可选")}</span></span><input data-action="decision.laya-key" type="password" value={layaKey} onChange={(e) => setLayaKey(e.target.value)} /></label>
    {error && <p className="note" data-lvl="warn">{error}</p>}
    <div className="provider-toolbar"><span /><button className="act" data-action="decision.save" data-primary disabled={busy} onClick={save}>{busy ? t("正在保存…") : t("保存")}</button></div>
  </>;
}
