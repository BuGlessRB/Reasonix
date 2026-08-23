import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import type { HubPort } from "../port/hub";
import type { RemoteHost, RemoteHostEdit } from "../port/remote";

interface Props {
  hub: HubPort;
  onError: (e: unknown) => void;
}

const STATUS_LABEL: Record<string, string> = {
  idle: "未连接",
  connecting: "连接中",
  connected: "已连上",
  reconnecting: "重连中",
  degraded: "有转发没挂上",
  stopped: "已断开",
};

// A saved row goes back in full: the endpoint replaces the entry, so a form
// that sent only what it displays would blank the rest.
function draftOf(host: RemoteHost | null): RemoteHostEdit {
  return {
    name: host?.name ?? "",
    host: host?.host ?? "",
    port: host?.port ?? 0,
    user: host?.user ?? "",
    identityFile: host?.identityFile ?? "",
    proxyJump: host?.proxyJump ?? "",
    workspace: host?.workspace ?? "",
    serveInstall: host?.serveInstall ?? "",
    useSSHConfig: host?.useSSHConfig ?? false,
    passphraseEnv: host?.passphraseEnv ?? "",
    passwordEnv: host?.passwordEnv ?? "",
  };
}

export function Remotes({ hub, onError }: Props) {
  const [hosts, setHosts] = useState<RemoteHost[]>([]);
  const [candidates, setCandidates] = useState<string[]>([]);
  const [draft, setDraft] = useState<RemoteHostEdit | null>(null);
  // The name a draft is editing, empty for a new one. Kept apart from the
  // draft's own name so renaming stays possible later without losing the row.
  const [editing, setEditing] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState("");

  const reload = useCallback(async () => {
    try {
      const [book, aliases] = await Promise.all([hub.remoteHosts(), hub.remoteCandidates().catch(() => [])]);
      setHosts(book ?? []);
      setCandidates(aliases);
    } catch (e) {
      onError(e);
    }
  }, [hub, onError]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const save = async (entry: RemoteHostEdit) => {
    setBusy(entry.name);
    try {
      await hub.saveRemoteHost(entry);
      setDraft(null);
      setEditing("");
      await reload();
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  const drop = async (name: string) => {
    setConfirm("");
    setBusy(name);
    try {
      await hub.removeRemoteHost(name);
      await reload();
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  const field = (key: keyof RemoteHostEdit, label: string, placeholder?: string) => (
    <label className="rmtf">
      <span>{t(label)}</span>
      <input
        value={String(draft?.[key] ?? "")}
        placeholder={placeholder ? t(placeholder) : undefined}
        onChange={(ev) => setDraft((d) => (d ? { ...d, [key]: ev.target.value } : d))}
      />
    </label>
  );

  return (
    <div className="rmtbook">
      {hosts.map((host) => (
        <div className="rmtrow" key={host.name} data-state={host.status}>
          <div className="hd">
            <i className="rmtpip" aria-hidden="true" />
            <b>{host.name}</b>
            <span className="tg">{host.target}</span>
            <span className="st">{t(STATUS_LABEL[host.status] ?? host.status)}</span>
            <button
              className="rmtlnk"
              onClick={() => {
                setEditing(host.name);
                setDraft(draftOf(host));
              }}
            >
              {t("编辑")}
            </button>
            <button className="rmtlnk" data-danger="" disabled={!!busy} onClick={() => setConfirm(host.name)}>
              {t("移除")}
            </button>
          </div>
          <div className="sub">
            {host.workspace ? <span dir="ltr">{host.workspace}</span> : <span className="dim">{t("没设默认工作区")}</span>}
            {host.useSSHConfig ? <span className="tag">{t("跟随 ssh_config")}</span> : null}
            {host.forwards ? <span className="tag">{t("{n} 条转发", { n: host.forwards })}</span> : null}
            {host.panes ? <span className="tag">{t("{n} 个面板", { n: host.panes })}</span> : null}
          </div>
          {confirm === host.name && (
            <div className="rmtconfirm" role="alertdialog">
              <span>{t("从列表移除「{name}」？远端什么都不会删。", { name: host.name })}</span>
              <button onClick={() => setConfirm("")}>{t("取消")}</button>
              <button data-danger="" autoFocus onClick={() => void drop(host.name)}>
                {t("移除")}
              </button>
            </div>
          )}
        </div>
      ))}

      {!hosts.length && !draft && <p className="rmtempty">{t("还没有远程机器。加一台，它的工作区就和本地的并排出现在左栏。")}</p>}

      {/* Importing beats typing: on a machine that already uses ssh, the
          addresses are written down next door and this only borrows the name. */}
      {candidates.length > 0 && (
        <div className="rmtcands">
          <span className="cap">{t("~/.ssh/config 里还有")}</span>
          {candidates.map((alias) => (
            <button
              key={alias}
              className="rmtcand"
              disabled={!!busy}
              title={t("按 ssh_config 里的设置加进来")}
              onClick={() => void save({ name: alias, useSSHConfig: true })}
            >
              + {alias}
            </button>
          ))}
        </div>
      )}

      {draft ? (
        <div className="rmtform">
          {field("name", "名字", "gpu-box")}
          {field("host", "地址", "10.0.0.4")}
          {field("user", "用户", "ada")}
          <label className="rmtf">
            <span>{t("端口")}</span>
            <input
              value={draft.port ? String(draft.port) : ""}
              placeholder="22"
              inputMode="numeric"
              onChange={(ev) => setDraft((d) => (d ? { ...d, port: Number(ev.target.value.replace(/\D/g, "")) || 0 } : d))}
            />
          </label>
          {field("workspace", "默认工作区", "/srv/training")}
          {field("identityFile", "私钥文件", "~/.ssh/id_ed25519")}
          {/* Named, not carried: this is the variable to read, never the secret
              itself, so nothing typed here is a password on its way anywhere. */}
          {field("passphraseEnv", "私钥口令的环境变量名", "GPU_BOX_PASSPHRASE")}
          <label className="rmtf rmtck">
            <input
              type="checkbox"
              checked={!!draft.useSSHConfig}
              onChange={(ev) => setDraft((d) => (d ? { ...d, useSSHConfig: ev.target.checked } : d))}
            />
            <span>{t("空着的项去 ~/.ssh/config 里找")}</span>
          </label>
          <div className="rmtact">
            <button
              onClick={() => {
                setDraft(null);
                setEditing("");
              }}
            >
              {t("取消")}
            </button>
            <button data-go="" disabled={!draft.name.trim() || !!busy} onClick={() => void save(draft)}>
              {editing ? t("保存") : t("添加")}
            </button>
          </div>
        </div>
      ) : (
        <button className="rmtadd" onClick={() => setDraft(draftOf(null))}>
          + {t("加一台机器")}
        </button>
      )}
    </div>
  );
}
