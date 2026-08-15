import { useState } from "react";
import type { ExtensionSurface } from "../port/wire";
import { ExtensionView } from "./cards/ExtensionView";
import { SLOTS, placement } from "./slots";

// A view plus the one control the host adds to it: where it goes. The handle is
// the host's, not the extension's — an extension asks for a place, and this is
// where the user overrules it. It stays hidden until the surface is hovered,
// because the common case is reading the thing, not moving it.
export function SlottedView({
  ext, assigned = {}, onAction, onMove,
}: {
  ext: ExtensionSurface;
  assigned?: Record<string, string>;
  onAction: (actionId: string) => void;
  onMove: (slot: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const here = placement(ext, assigned);
  return (
    <div className="slotted">
      <ExtensionView body={ext.view?.body ?? []} onAction={onAction} />
      <button
        className="movegrip"
        aria-label={`移动 ${ext.pluginId} 的界面`}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        ⋯
      </button>
      {open && (
        <div className="movemenu" role="menu">
          <span className="who">{ext.pluginId}</span>
          {SLOTS.map((slot) => (
            <button
              key={slot.id}
              role="menuitemradio"
              aria-checked={here === slot.id}
              onClick={() => {
                setOpen(false);
                onMove(slot.id);
              }}
            >
              {slot.label}
            </button>
          ))}
          {/* Clearing hands the choice back to the extension rather than
              hiding the surface: it is a different act from moving it. */}
          <button role="menuitem" onClick={() => { setOpen(false); onMove(""); }}>
            交还给插件
          </button>
        </div>
      )}
    </div>
  );
}
