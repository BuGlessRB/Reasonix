// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import "./testkit";
import type { AgentPort } from "../port/port";
import type { HubPort } from "../port/hub";
import type { StorageQuery, StorageRoot, StorageState } from "../port/storage";
import { Storage } from "./Storage";

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const root = (id: string, over: Partial<StorageRoot> = {}): StorageRoot => ({
  id, dir: `/data/${id}`, bytes: 0, files: 0, relocatable: false, ...over,
});
const state = (roots: StorageRoot[]): StorageState => ({ roots, editable: false });

function draw(answer: (q?: StorageQuery) => Promise<StorageState>) {
  const port = { storage: vi.fn(answer) } as unknown as AgentPort;
  render(<Storage port={port} hub={{} as HubPort} workspace="" onRecovered={() => {}} />);
  return port;
}

const flush = () => act(async () => { await vi.advanceTimersByTimeAsync(0); });

it("shows fast roots while a slow one is still being measured", async () => {
  draw(async (q) => {
    if (q?.layout) return state([root("state", { pending: true }), root("cache", { pending: true })]);
    if (q?.root === "cache") return state([root("cache", { bytes: 2048, files: 4 })]);
    return new Promise(() => {});
  });
  await flush();
  expect(screen.getByText(/2(\.0)? KB · 4 个文件/)).toBeTruthy();
  expect(screen.getAllByText("正在统计…")).toHaveLength(1);
});

it("says a root timed out instead of spinning forever", async () => {
  draw(async (q) => {
    if (q?.layout) return state([root("state", { pending: true })]);
    return new Promise(() => {});
  });
  await flush();
  expect(screen.getByText("正在统计…")).toBeTruthy();
  await act(async () => { await vi.advanceTimersByTimeAsync(21_000); });
  expect(screen.getByText("统计超时")).toBeTruthy();
  expect(screen.queryByText("正在统计…")).toBeNull();
});

it("marks a truncated count as a lower bound", async () => {
  draw(async (q) => {
    if (q?.layout) return state([root("state", { pending: true })]);
    return state([root("state", { bytes: 3072, files: 9, truncated: true })]);
  });
  await flush();
  expect(screen.getByText(/^≥ 3(\.0)? KB · 9 个文件/)).toBeTruthy();
  expect(screen.getByText("目录过大，已统计到时间上限，实际占用不小于此数")).toBeTruthy();
});
