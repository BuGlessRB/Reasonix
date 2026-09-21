// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "./testkit";
import type { Port } from "./Providers";
import { AddProvider } from "./AddProvider";

afterEach(cleanup);

it("saves an explicitly configured source without requiring protocol detection", async () => {
  const saveProvider = vi.fn(async () => {});
  const probeProvider = vi.fn();
  const port = {
    protocols: vi.fn(async () => [
      { kind: "openai", discovery: "openai", serverWebSearch: false, reasoningParams: true },
      { kind: "anthropic", discovery: "anthropic", serverWebSearch: true, reasoningParams: true },
    ]),
    saveProvider,
    probeProvider,
  } as unknown as Port;

  render(<AddProvider port={port} taken={[]} known={[]} onDone={() => {}} onCancel={() => {}} />);
  await userEvent.type(screen.getByLabelText("来源名称"), "my-relay");
  await userEvent.selectOptions(await screen.findByLabelText("接口协议"), "anthropic");
  await userEvent.type(screen.getByLabelText("接口地址"), "https://relay.example/v1");
  await userEvent.type(screen.getByLabelText("上下文窗口"), "200000");
  await userEvent.type(screen.getByLabelText("最大输出"), "32000");
  await userEvent.type(screen.getByRole("searchbox", { name: "搜索或添加模型" }), "claude-custom{enter}");
  await userEvent.click(screen.getByRole("button", { name: "添加来源" }));

  await waitFor(() => expect(saveProvider).toHaveBeenCalled());
  expect(probeProvider).not.toHaveBeenCalled();
  expect(saveProvider).toHaveBeenCalledWith(expect.objectContaining({
    name: "my-relay",
    kind: "anthropic",
    models: ["claude-custom"],
    contextWindow: 200000,
    maxOutputTokens: 32000,
  }));
});

it("keeps relay-only controls optional and saves them when disclosed", async () => {
  const saveProvider = vi.fn(async () => {});
  const port = {
    protocols: vi.fn(async () => [
      { kind: "openai", discovery: "openai", serverWebSearch: false, reasoningParams: true },
    ]),
    saveProvider,
  } as unknown as Port;

  render(<AddProvider port={port} taken={[]} known={[]} onDone={() => {}} onCancel={() => {}} />);
  await userEvent.type(screen.getByLabelText("来源名称"), "relay");
  await userEvent.type(screen.getByLabelText("接口地址"), "https://relay.example/v1");
  await userEvent.type(screen.getByRole("searchbox", { name: "搜索或添加模型" }), "model-x{enter}");
  await userEvent.click(screen.getByText("高级连接选项"));
  await userEvent.click(screen.getByRole("switch", { name: "发送思考控制" }));
  await userEvent.type(screen.getByText("额外请求头").parentElement!.querySelector("textarea")!, "X-Title: Reasonix");
  await userEvent.click(screen.getByRole("button", { name: "添加来源" }));

  await waitFor(() => expect(saveProvider).toHaveBeenCalled());
  expect(saveProvider).toHaveBeenCalledWith(expect.objectContaining({
    reasoningProtocol: "none",
    headers: { "X-Title": "Reasonix" },
  }));
});
