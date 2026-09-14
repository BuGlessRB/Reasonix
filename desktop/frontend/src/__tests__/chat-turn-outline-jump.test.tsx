import assert from "node:assert/strict";
import { act } from "react";
import { createTranscriptHarness } from "./transcript-dom-harness";
import type { TranscriptOutlineStore } from "../lib/transcriptOutlineStore";
import type { TranscriptOutlineEntry, TranscriptOutlinePage } from "../lib/transcriptProtocol";
import type { Item } from "../lib/useController";

const TAB = "outline-jump-tab";
const SNAPSHOT = "snapshot-1";
const TOTAL = 6;

function entry(turn: number): TranscriptOutlineEntry {
  return {
    id: `m:u${turn}`, messageId: `u${turn}`, turn, order: turn * 2 - 2,
    prompt: `outline prompt ${turn}`, answer: `outline answer ${turn}`,
  };
}

function outlinePage(): TranscriptOutlinePage {
  return {
    protocolVersion: 1, snapshotId: SNAPSHOT, stale: false,
    entries: Array.from({ length: TOTAL }, (_, index) => entry(index + 1)),
    nextOffset: TOTAL, done: true, total: TOTAL,
  };
}

/** The loaded body window: the newest turns, oldest first, as the transcript
 * loads them — an earlier page prepends above this. */
function turnsFrom(start: number): Item[] {
  const items: Item[] = [];
  for (let index = start; index <= TOTAL; index += 1) {
    items.push({ kind: "user", id: `m:u${index}`, text: `loaded prompt ${index}`, historyTurn: index });
    items.push({ kind: "assistant", id: `a${index}`, text: `loaded answer ${index}`, reasoning: "", streaming: false });
  }
  return items;
}

const harness = await createTranscriptHarness({ deterministic: true });
let store: TranscriptOutlineStore | undefined;
try {
  await harness.loadModule("/src/components/ChatToolBody.tsx");
  // The component resolves the store through the harness's module graph, so the
  // test must take the same singleton instance rather than its own import.
  const outlineModule = await harness.loadModule<{
    getTranscriptOutlineStore: () => TranscriptOutlineStore;
  }>("/src/lib/transcriptOutlineStore.ts");
  store = outlineModule.getTranscriptOutlineStore();
  store.register(TAB, async () => outlinePage());
  await store.sync(TAB, SNAPSHOT);
  assert.equal(store.getView(TAB).mode, "ready", "the outline is bound to the snapshot");

  // Only the newest two turns are loaded. The rail must still show the whole
  // conversation, which is the reported defect.
  let windowStart = TOTAL - 1;
  let pages = 0;
  const render = () => harness.render(turnsFrom(windowStart), {
    tabId: TAB, totalTurns: TOTAL, hasOlderHistory: windowStart > 1, historyStartTurn: windowStart - 1,
    onLoadOlderHistory: async () => {
      pages += 1;
      if (windowStart <= 1) return false;
      windowStart = Math.max(1, windowStart - 2);
      await render();
      return true;
    },
  });
  await render();
  await harness.settle();

  const marks = () => Array.from(harness.container.querySelectorAll<HTMLElement>("[data-nav-turn]"));
  // The rail is a lazily imported chunk, so let it commit before asserting.
  await harness.waitFor(() => marks().length === TOTAL, "the rail to list the complete outline");
  assert.equal(marks().length, TOTAL, "the rail lists every turn, not only the loaded ones");
  assert.deepEqual(
    marks().map(mark => mark.dataset.navTurn),
    Array.from({ length: TOTAL }, (_, index) => `m:u${index + 1}`),
    "rail order follows the complete conversation",
  );
  const unloaded = marks().filter(mark => mark.dataset.navUnloaded === "true");
  assert.deepEqual(unloaded.map(mark => mark.dataset.navTurn), ["m:u1", "m:u2", "m:u3", "m:u4"],
    "turns without a mounted body are marked unloaded");
  assert.equal(harness.container.querySelector('[data-chat-anchor-key="m:u1"]'), null, "the oldest turn is not loaded yet");

  // Absolute turn numbering must survive loading an earlier page.
  const labelsBefore = marks().map(mark => mark.getAttribute("aria-label"));
  assert.match(labelsBefore[0]!, /1/, "the first mark is turn 1");

  // Clicking an unloaded turn pages history in until its node is mounted.
  const targetMark = marks().find(mark => mark.dataset.navTurn === "m:u1")!;
  await act(async () => { targetMark.click(); });
  await harness.waitFor(
    () => harness.container.querySelector('[data-chat-anchor-key="m:u1"]') !== null,
    "the oldest turn's node to mount",
  );
  await harness.settle();
  assert.ok(pages >= 2, "the jump paged older history more than once");
  assert.equal(
    marks().find(mark => mark.dataset.navTurn === "m:u1")?.dataset.navUnloaded,
    undefined,
    "the target is no longer marked unloaded once mounted",
  );
  assert.deepEqual(
    marks().map(mark => mark.dataset.navTurn),
    Array.from({ length: TOTAL }, (_, index) => `m:u${index + 1}`),
    "the rail keeps its identity and order after the jump",
  );
  assert.deepEqual(marks().map(mark => mark.getAttribute("aria-label")), labelsBefore,
    "loading an earlier page never renumbers the rail");

  // The rail's busy state clears once the target is reached.
  await harness.waitFor(
    () => harness.container.querySelectorAll('[aria-busy="true"]').length === 0,
    "the busy state to clear",
  );

  console.log("chat turn outline jump: complete rail, unloaded marks, absolute numbering and mount-confirmed jump passed");
} finally {
  store?.release(TAB);
  await harness.unmount();
  await harness.close();
}
