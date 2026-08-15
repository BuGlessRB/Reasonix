// The one on/off control in the settings pane: a durable switch, not a
// command. Everything that can be turned back on by clicking again is one.
export function Switch({
  on, busy, label, onClick,
}: {
  on: boolean; busy?: boolean; label: string; onClick: () => void;
}) {
  return (
    <button className="sw" role="switch" aria-checked={on} aria-label={label} disabled={busy} onClick={onClick}>
      <i />
    </button>
  );
}
