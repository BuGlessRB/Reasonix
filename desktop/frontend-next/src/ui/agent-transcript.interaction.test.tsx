// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import "./testkit";
import { DeckChips, type Deck } from "./DeckChips";
import { AgentTranscript } from "./panels/AgentTranscript";
import { NestedCall } from "./cards/ToolCard";
import type { Task } from "./panels/Agents";
import type { Tool } from "../port/wire";

afterEach(cleanup);

const LONG = "line-".repeat(120);
const child = (id: string, output: string): Tool => ({ id, name: "read_file", args: '{"path":"a.go"}', output, parentId: "t1" }) as Tool;
const task = (id: string, running: boolean, name: string, children: Tool[] = []): Task =>
  ({ t: "tool", id, running, children, tool: { id, name: "task", profile: { name }, output: running ? undefined : "final report", durationMs: 4000 } }) as unknown as Task;

function Host({ tasks }: { tasks: Task[] }) {
  const [open, setOpen] = useState<Deck>("agents");
  return <DeckChips tasks={tasks} jobs={[]} open={open} onOpen={(next) => setOpen(next)}/>;
}

it("lists running and finished sub-agents as separate rows and opens the one clicked", async () => {
  render(<Host tasks={[task("a", true, "Explore"), task("b", false, "Review")]}/>);
  const rows = document.querySelectorAll<HTMLElement>(".ag");
  expect([...rows].map((r) => r.dataset.status)).toEqual(["running", "done"]);
  await userEvent.click(screen.getByRole("button", { name: "查看完整记录：Review" }));
  const dialog = screen.getByRole("dialog", { name: "子代理完整记录：Review" });
  expect(dialog.textContent).toContain("final report");
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog", { name: "子代理完整记录：Review" })).toBeNull();
});

it("keeps the sub-agent reading after every delegate has finished", () => {
  render(<Host tasks={[task("b", false, "Review")]}/>);
  const chip = document.querySelector('[data-action="deck.agents"]')!;
  expect(chip).not.toBeNull();
  expect(chip.hasAttribute("data-live")).toBe(false);
});

it("shows the whole transcript of one sub-agent, unclipped, and closes on Escape", async () => {
  const onClose = vi.fn();
  render(<AgentTranscript task={task("b", false, "Review", [child("c1", LONG), child("c2", "short")])} onClose={onClose} />);
  const dialog = screen.getByRole("dialog");
  expect(dialog.textContent).toContain(LONG);
  expect(dialog.textContent).toContain("final report");
  await userEvent.keyboard("{Escape}");
  expect(onClose).toHaveBeenCalled();
});

it("offers show-all instead of silently cutting a long nested output", async () => {
  render(<NestedCall tool={child("c1", LONG)} />);
  expect(document.body.textContent).not.toContain(LONG);
  await userEvent.click(screen.getByRole("button", { name: "显示全部" }));
  expect(document.body.textContent).toContain(LONG);
  await userEvent.click(screen.getByRole("button", { name: "收起" }));
  expect(document.body.textContent).not.toContain(LONG);
});

it("offers no show-all for an output that fits", () => {
  render(<NestedCall tool={child("c1", "short")} />);
  expect(screen.queryByRole("button", { name: "显示全部" })).toBeNull();
});
