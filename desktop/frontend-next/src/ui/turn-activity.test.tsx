// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import "./testkit";
import { Transcript } from "./Transcript";
import type { Item } from "../state/session";

afterEach(cleanup);

const user = (id: string, text: string): Item => ({ t: "user", id, text });
const say = (id: string, text: string): Item => ({ t: "say", id, text, done: true });
const call = (id: string, name: string): Item => ({ t: "tool", id, tool: { id, name, readOnly: true }, running: false, children: [] });

const noop = async () => undefined as never;

function draw(items: Item[]) {
  render(
    <Transcript
      items={items}
      entering={[]}
      onEntered={() => {}}
      revision={1}
      waiting={{}}
      scroll={{ current: null }}
      hidden={false}
      onPinned={() => {}}
      jump={0}
      focus={null}
      onApprove={noop}
      onFullAccess={noop}
      onPlan={noop}
      onAnswer={noop}
      onForget={noop}
      onCancelQueued={() => {}}
      onExtInvoke={() => {}}
      onExtSubmit={noop}
      checkpoints={new Map()}
      onPrepareRewind={noop}
      onCommitRewind={noop}
      onUndoRewind={noop}
      onPrepareFileRevert={noop}
      onCommitFileRevert={noop}
      needsProject={false}
      onOpenProject={() => {}}
      onKeepHere={() => {}}
    />,
  );
  return document.querySelector(".chunk") as HTMLElement;
}

// A turn answers more than once: the model speaks, works, speaks again. Each
// stretch of work belongs to the sentence that preceded it, so a reader finds
// the command under the thing it was for.
describe("the work under a turn's sentences", () => {
  it("opens a disclosure per stretch of work", () => {
    const chunk = draw([
      user("u", "做这件事"),
      say("s1", "先看看代码"),
      call("t1", "bash"),
      call("t2", "read_file"),
      say("s2", "现在改这里"),
      call("t3", "edit_file"),
      say("s3", "改好了"),
    ]);

    const groups = [...chunk.querySelectorAll(".activity-group")];
    expect(groups).toHaveLength(2);
    expect(groups[0].textContent).toContain("bash");
    expect(groups[1].textContent).toContain("edit_file");
  });

  it("keeps each sentence above the work that followed it", () => {
    const chunk = draw([
      user("u", "做这件事"),
      say("s1", "先看看代码"),
      call("t1", "bash"),
      say("s2", "现在改这里"),
      call("t2", "edit_file"),
      say("s3", "改好了"),
    ]);

    const shape = [...chunk.querySelectorAll<HTMLElement>('[data-k="say"], .activity-group')].map((el) => {
      if (el.classList.contains("activity-group")) return "work";
      const text = el.textContent ?? "";
      return text.includes("先看看代码") ? "s1" : text.includes("现在改这里") ? "s2" : "s3";
    });
    expect(shape).toEqual(["s1", "work", "s2", "work", "s3"]);
  });
});
