// @vitest-environment jsdom
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { HubPort } from "../port/hub";
import { useAddWorkspace } from "./addws";

function hub(pickFolder: () => Promise<string | null>) {
  return { pickFolder, addWorkspace: vi.fn(async () => undefined) } as unknown as HubPort;
}

describe("adding a workspace", () => {
  it("adds the folder returned by the native picker", async () => {
    const port = hub(async () => "D:\\work\\project");
    const reload = vi.fn(async () => undefined);
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, reload, fail));

    act(() => result.current.add());

    await waitFor(() => expect(port.addWorkspace).toHaveBeenCalledWith("D:\\work\\project"));
    expect(reload).toHaveBeenCalledOnce();
    expect(fail).not.toHaveBeenCalled();
  });

  it("explains the browser limitation instead of asking for a path", async () => {
    const port = hub(async () => null);
    const fail = vi.fn();
    const { result } = renderHook(() => useAddWorkspace(port, vi.fn(), fail));

    act(() => result.current.add());

    await waitFor(() => expect(fail).toHaveBeenCalledOnce());
    expect(String(fail.mock.calls[0]?.[0])).toContain("桌面端选择工作区");
    expect(port.addWorkspace).not.toHaveBeenCalled();
  });
});
