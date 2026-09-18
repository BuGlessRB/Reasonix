import { describe, expect, it } from "vitest";

// The pane's navigation changed; what it navigates between did not. These are
// the parts of Pane.tsx that a tidier bar makes tempting to unify — and each of
// them is a contract rather than an implementation detail, which is why they are
// read off the source instead of being left to a reviewer to notice.
const SOURCE = import.meta.glob("./Pane.tsx", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const DETAIL = import.meta.glob("./PaneDetail.tsx", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const pane = Object.values(SOURCE)[0].replace(/\s+/g, " ");
// The three diagnostic readings live in their own file; the mounting contract
// is theirs wherever they are drawn from.
const detail = Object.values(DETAIL)[0].replace(/\s+/g, " ");

describe("what the pane still owns after the bar stopped listing it", () => {
  // Hiding a diagnostic view with an attribute left every row of it being
  // rebuilt on each streamed delta — a second transcript's worth of work drawn
  // for nobody. Extracting one <View /> to make the markup symmetric is exactly
  // how that comes back, so the asymmetry is the contract.
  it.each(["line", "traj", "graph"])("mounts %s only while it is the view on screen", (view) => {
    expect(detail).toMatch(new RegExp(`\\{ ?view === "${view}" &&`));
  });

  it("mounts the task board only while it is the view on screen", () => {
    expect(pane).toMatch(/\{ ?tab === "task" &&/);
  });

  // The browser is the one that may also be drawn beside the transcript, so it
  // is mounted for either reading and for neither otherwise.
  it("mounts the browser for the tab or for the dock beside it, and not otherwise", () => {
    expect(pane).toMatch(/\{\(docked \|\| tab === "browser"\) && \(/);
    expect(pane).toMatch(/const docked = dock && pages\.length > 0 && tab !== "browser"/);
  });

  // The transcript is the exception on purpose: it keeps its scroll position,
  // its pinning and its entrance bookkeeping across a view change.
  it("keeps the transcript mounted and merely hidden", () => {
    expect(pane).toMatch(/hidden=\{ ?tab !== "flow" ?\}/);
  });

  it("still holds the five view identities rather than a collapsed three", () => {
    expect(pane).toMatch(/useState<PaneView>\("flow"\)/);
    for (const view of ["flow", "line", "traj", "graph", "task", "browser"]) {
      expect(`${pane} ${detail}`).toContain(`"${view}"`);
    }
    // A second selection saying which detail is open would be a second answer
    // to a question the view identity already answers.
    expect(pane).not.toMatch(/useState[^;]*"details"/);
  });
});

describe("the routes into a view that never went through the bar", () => {
  it("still lands a graph or timeline node on its call in the transcript", () => {
    expect(pane).toMatch(/setTab\("flow"\); setFocus/);
  });

  it("still sends the task board to the trajectory and to the latest turn", () => {
    expect(pane).toMatch(/onTrajectory=\{\(\) => swapping\(\(\) => setTab\("traj"\)/);
    expect(pane).toMatch(/onLatest=\{\(\) => \{ swapping\(\(\) => setTab\("flow"\), "tab"\); toLatest\(\);/);
  });

  // A run that loses its graph takes the timeline with it, and a reader sitting
  // on the timeline has to be sent somewhere that exists.
  it("still leaves the timeline when the run has no graph left", () => {
    expect(pane).toMatch(/tab === "line" && exec\.graph\.nodes\.length === 0\) setTab\("flow"\)/);
  });

  // The same for the browser: a reader on it when the agent's last page closes
  // is sent back to the transcript rather than left on an empty view.
  it("leaves the browser when the agent has no page left", () => {
    expect(pane).toMatch(/tab === "browser" && pages\.length === 0\) setTab\("flow"\)/);
  });

  // One route, so a view reached from the bar, from the menu, from the task
  // board and from a graph node is one motion rather than four.
  it("changes views through the shared transition and not around it", () => {
    expect(pane).toMatch(/showView = useCallback\(\(to: PaneView\) => swapping\(\(\) => setTab\(to\), "tab"\)/);
  });
});
