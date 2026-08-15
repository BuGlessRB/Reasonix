import type { ExtensionSurface } from "../../port/wire";

// Standing surfaces an extension published. They live here rather than in the
// transcript because they describe a state that is still true — a watcher, a
// sync, a connection — which scrolling away would hide.
export function Extensions({
  panels,
  onInvoke,
}: {
  panels: ExtensionSurface[];
  onInvoke: (name: string) => void;
}) {
  if (panels.length === 0) return null;
  return (
    <div className="block" data-b="extpanels">
      <div className="lbl">
        扩展<span className="c">{panels.length}</span>
      </div>
      {panels.map((p) => {
        const panel = p.panel;
        if (!panel) return null;
        return (
          <div className="extpanel" key={`${p.pluginId}:${p.surfaceId}`}>
            <div className="extpanel-hd">
              <span className="nm">{panel.title || p.pluginId}</span>
              {panel.title && <span className="src">{p.pluginId}</span>}
            </div>
            {panel.text && <div className="extpanel-tx">{panel.text}</div>}
            {panel.progress !== undefined && (
              <span className="extbar" role="progressbar" aria-valuenow={Math.round(panel.progress * 100)}>
                <i style={{ width: `${Math.max(0, Math.min(1, panel.progress)) * 100}%` }} />
              </span>
            )}
            {!!panel.fields?.length && (
              <dl className="extkv">
                {panel.fields.map((f) => (
                  <div key={f.key}>
                    <dt>{f.key}</dt>
                    <dd>{f.value}</dd>
                  </div>
                ))}
              </dl>
            )}
            {!!panel.actions?.length && (
              <div className="extacts">
                {panel.actions.map((a) => (
                  <button
                    key={a.actionId}
                    className="btn"
                    onClick={() => onInvoke(`/${p.pluginId}:${a.actionId}`)}
                  >
                    {a.label || a.actionId}
                  </button>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
