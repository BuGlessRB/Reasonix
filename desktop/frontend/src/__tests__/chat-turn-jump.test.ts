import assert from "node:assert/strict";
import { ChatMountedOrder } from "../lib/chatMountedOrder";
import { ChatTurnJump } from "../lib/chatTurnJump";
import type { ChatScrollController } from "../lib/chatScrollController";
import { alignOutlineEntries, loadedTurnKey } from "../lib/chatTurnRail";
import type { TranscriptOutlineEntry } from "../lib/transcriptProtocol";

// Node has no animation frame; the mount-settle path is driven by the mounted
// store in these tests, so a recorded no-op keeps it deterministic.
const frames: FrameRequestCallback[] = [];
(globalThis as unknown as { requestAnimationFrame: (cb: FrameRequestCallback) => number }).requestAnimationFrame =
  (callback) => frames.push(callback);
(globalThis as unknown as { cancelAnimationFrame: (id: number) => void }).cancelAnimationFrame = () => {};

function flushFrames(): void {
  const pending = frames.splice(0);
  for (const callback of pending) callback(0);
}

/** Records the writes a jump asks the shared gateway to make. */
function fakeScroll() {
  const jumps: string[] = [];
  const readers = new Set<() => void>();
  let stopped = false;
  const scroll = {
    stopFollowing: () => { stopped = true; },
    jump: (key: string) => { jumps.push(key); },
    subscribeReaderIntent: (listener: () => void) => { readers.add(listener); return () => { readers.delete(listener); }; },
    subscribe: () => () => {},
    getSnapshot: () => ({ following: false, activeKey: "" }),
  };
  return {
    scroll: scroll as unknown as ChatScrollController,
    jumps,
    stopped: () => stopped,
    // Reader intent is what a wheel, touch, key press, or return-to-bottom
    // reports through the controller's dedicated channel.
    readerIntent: () => { for (const listener of [...readers]) listener(); },
    readerCount: () => readers.size,
  };
}

function jumpFor(mounts: ChatMountedOrder, options: {
  mounted: Set<string>;
  pages?: string[][];
  hasOlder?: () => boolean;
  current?: () => boolean;
}) {
  const fake = fakeScroll();
  let pageIndex = 0;
  const loads: number[] = [];
  const jump = new ChatTurnJump({
    mounts,
    scroll: fake.scroll,
    loadOlder: async () => {
      loads.push(pageIndex);
      const revealed = options.pages?.[pageIndex] ?? [];
      pageIndex += 1;
      for (const key of revealed) options.mounted.add(key);
      // One page also advances the progressive mount.
      mounts.publish([...options.mounted]);
      return revealed.length > 0;
    },
    hasOlder: options.hasOlder ?? (() => true),
    resolveKey: (entry) => (options.mounted.has(entry.id) ? entry.id : undefined),
    isCurrent: options.current ?? (() => true),
  });
  return { jump, fake, loads, mounted: options.mounted };
}

function target(id: string): TranscriptOutlineEntry {
  return { id, messageId: id.replace("m:", ""), turn: 1, order: 0, prompt: "p", answer: "a" };
}

async function main() {
  {
    // An already-mounted target must not page at all.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(["m:1"]) });
    await state.jump.jump(target("m:1"));
    assert.deepEqual(state.fake.jumps, ["m:1"], "a loaded target scrolls immediately");
    assert.deepEqual(state.loads, [], "a loaded target does not page history");
    assert.equal(state.jump.getSnapshot().status, "idle", "the jump completes");
    assert.ok(state.fake.stopped(), "a jump leaves tail following before it writes");
  }

  {
    // An unloaded target pages until its node is really mounted, then scrolls.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(["m:9"]), pages: [["m:5"], ["m:3"], ["m:1"]] });
    await state.jump.jump(target("m:1"));
    assert.deepEqual(state.loads, [0, 1, 2], "history is paged one batch at a time");
    assert.deepEqual(state.fake.jumps, ["m:1"], "the write happens only after the node mounts");
    assert.equal(state.jump.getSnapshot().status, "idle");
  }

  {
    // Exhausting history without reaching the target is a specific failure.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(["m:9"]), pages: [["m:9"], ["m:8"]] });
    await state.jump.jump(target("m:404"));
    assert.deepEqual(state.fake.jumps, [], "an unreachable target never moves the viewport");
    assert.equal(state.jump.getSnapshot().status, "failed");
    assert.equal(state.jump.getSnapshot().error, "turnUnavailable");
  }

  {
    // No older history at all fails immediately instead of looping.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(), hasOlder: () => false });
    await state.jump.jump(target("m:404"));
    assert.deepEqual(state.loads, [], "a missing target with no history is not retried");
    assert.equal(state.jump.getSnapshot().status, "failed");
  }

  {
    // A page that adds nothing stops the loop rather than spinning the network.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(), pages: [[]] });
    await state.jump.jump(target("m:404"));
    assert.equal(state.loads.length, 1, "an unproductive page ends the jump");
    assert.equal(state.jump.getSnapshot().status, "failed");
  }

  {
    // Reader intent preempts a pending jump and no later page takes the viewport.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(), pages: [["m:5"], ["m:1"]] });
    const pending = state.jump.jump(target("m:1"));
    await Promise.resolve();
    state.fake.readerIntent();
    await pending;
    assert.deepEqual(state.fake.jumps, [], "a preempted jump never scrolls");
    assert.equal(state.loads.length, 1, "no page is requested after the reader takes over");
    assert.equal(state.jump.getSnapshot().status, "idle", "preemption clears the busy state");
    assert.equal(state.fake.readerCount(), 0, "the reader subscription is released");
  }

  {
    // A newer target supersedes the pending one; only the newest may scroll.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(), pages: [["m:5"], ["m:2"], ["m:7"]] });
    const first = state.jump.jump(target("m:1"));
    await Promise.resolve();
    const second = state.jump.jump(target("m:7"));
    await Promise.all([first, second]);
    assert.deepEqual(state.fake.jumps, ["m:7"], "only the newest target takes scroll control");
    assert.equal(state.jump.getSnapshot().status, "idle");
  }

  {
    // A replaced session leaves no late callback able to move the viewport.
    const mounts = new ChatMountedOrder();
    let current = true;
    const state = jumpFor(mounts, { mounted: new Set(), pages: [["m:5"], ["m:1"]], current: () => current });
    const pending = state.jump.jump(target("m:1"));
    await Promise.resolve();
    current = false;
    await pending;
    assert.deepEqual(state.fake.jumps, [], "a replaced session cannot take scroll control back");
    assert.equal(state.loads.length, 1, "no further page is requested for a replaced session");
  }

  {
    // Cancelling explicitly ends the pending transaction.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set(), pages: [["m:5"], ["m:1"]] });
    const pending = state.jump.jump(target("m:1"));
    await Promise.resolve();
    state.jump.cancel();
    await pending;
    assert.deepEqual(state.fake.jumps, [], "a cancelled jump never scrolls");
    assert.equal(state.jump.getSnapshot().status, "idle");
  }

  {
    // The bounded frame budget resolves the settle wait even when nothing
    // publishes, so a jump cannot hang on an idle mount.
    const mounts = new ChatMountedOrder();
    const state = jumpFor(mounts, { mounted: new Set() });
    let pages = 0;
    const jump = new ChatTurnJump({
      mounts,
      scroll: state.fake.scroll,
      loadOlder: async () => { pages += 1; return true; },
      hasOlder: () => pages < 3,
      resolveKey: () => undefined,
      isCurrent: () => true,
    });
    let settled = false;
    const pending = jump.jump(target("m:1")).then(() => { settled = true; });
    // Drive microtasks and frames together: the settle wait must expire on its
    // own frame budget even though nothing ever publishes a mount.
    for (let i = 0; i < 20_000 && !settled; i++) {
      await Promise.resolve();
      flushFrames();
    }
    await pending;
    assert.ok(settled, "the mount wait is bounded and the jump terminates");
    assert.equal(pages, 3, "paging stops as soon as history is exhausted");
    assert.equal(jump.getSnapshot().status, "failed", "an unreachable target ends as a failure, not a hang");
  }

  {
    // Identity resolution: a settled question is found by its message ID, and
    // a question that has not been committed is found by its record ID. Both
    // must resolve to the node the rail would scroll to.
    const settled: TranscriptOutlineEntry = { id: "m:abc", messageId: "abc", turn: 1, order: 0, prompt: "", answer: "" };
    assert.equal(loadedTurnKey(settled, new Set(["m:abc"])), "m:abc", "a settled question resolves by message ID");
    assert.equal(loadedTurnKey(settled, new Set()), undefined, "an unmounted question has no key");

    const optimistic: TranscriptOutlineEntry = { id: "m:tmp", turn: 2, order: 2, prompt: "", answer: "" };
    assert.equal(loadedTurnKey(optimistic, new Set(["m:tmp"])), "m:tmp", "an uncommitted question resolves by record ID");
    assert.equal(loadedTurnKey(optimistic, new Set(["m:other"])), undefined, "an unrelated node is not claimed");

    // A message ID wins over a shadowing record ID, so settlement cannot move
    // a mark that already points at the canonical node.
    assert.equal(loadedTurnKey(settled, new Set(["m:abc", "m:abc:legacy"])), "m:abc", "message ID outranks the record ID fallback");

    const aligned = alignOutlineEntries([
      { id: "m:b", turn: 2, order: 2, prompt: "", answer: "" },
      { id: "m:a", turn: 1, order: 0, prompt: "", answer: "" },
      { id: "m:b", turn: 2, order: 2, prompt: "duplicate", answer: "" },
    ]);
    assert.deepEqual(aligned.map(item => item.id), ["m:a", "m:b"], "entries are ordered by snapshot position and de-duplicated");
  }

  console.log("chat turn jump: mount-confirmed paging, preemption, supersession, replacement, cancel and rail identity passed");
}

await main();
