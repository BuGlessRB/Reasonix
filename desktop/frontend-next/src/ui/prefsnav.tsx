import type { ReactNode } from "react";

/** The settings table of contents: which sections exist, what each is called,
 *  what mark it carries and which question it answers. Kept out of Settings
 *  itself because it is a table, not a screen — the screen reads it. */
export type Section = "session" | "model" | "tools" | "hooks" | "ext" | "network" | "remote" | "memory" | "usage" | "storage" | "account" | "versions" | "appearance" | "advanced";

// Drawn on the same 16-unit grid at 1.45 stroke as the rest of this screen's
// marks. The rail in Nav.tsx keeps its own set on purpose: those name panes to
// open, these name sections of one page, and the two lists barely overlap.
export const ICON: Record<Section, ReactNode> = {
  session: <path d="M2.6 8h10.8M8 2.6v10.8" />,
  model: (
    <>
      <circle cx="8" cy="8" r="2.4" />
      <path d="M8 1.8v2.2M8 12v2.2M1.8 8h2.2M12 8h2.2" />
    </>
  ),
  tools: (
    <>
      <path d="M8 1.9 13.8 4.4v4.2c0 3-2.4 5.1-5.8 5.7-3.4-.6-5.8-2.7-5.8-5.7V4.4Z" />
      <path d="M6.4 8.1 7.6 9.3l2.4-2.4" />
    </>
  ),
  hooks: (
    <>
      <path d="M4.4 2.6v6.2a3.2 3.2 0 0 0 6.4 0V6.2" />
      <circle cx="10.8" cy="4.4" r="1.6" />
    </>
  ),
  ext: (
    <>
      <rect x="2.4" y="2.4" width="5" height="5" rx="1.2" />
      <rect x="8.6" y="8.6" width="5" height="5" rx="1.2" />
      <path d="M7.4 5h6.2M5 7.4v6.2" />
    </>
  ),
  network: (
    <>
      <circle cx="8" cy="8" r="5.8" />
      <path d="M2.4 8h11.2M8 2.2c1.6 1.7 2.4 3.6 2.4 5.8S9.6 12.1 8 13.8C6.4 12.1 5.6 10.2 5.6 8s.8-4.1 2.4-5.8Z" />
    </>
  ),
  remote: (
    <>
      <rect x="2.2" y="2.6" width="11.6" height="4.4" rx="1.3" />
      <rect x="2.2" y="9" width="11.6" height="4.4" rx="1.3" />
      <path d="M4.8 4.8h.01M4.8 11.2h.01" />
    </>
  ),
  storage: (
    <>
      <ellipse cx="8" cy="4" rx="5.4" ry="2.1" />
      <path d="M2.6 4v8c0 1.2 2.4 2.1 5.4 2.1s5.4-.9 5.4-2.1V4" />
      <path d="M2.6 8c0 1.2 2.4 2.1 5.4 2.1s5.4-.9 5.4-2.1" />
    </>
  ),
  memory: <path d="M8 3.2c-1.4-1.3-4.6-1-4.6 1.9 0 2.4 2.6 4.4 4.6 6 2-1.6 4.6-3.6 4.6-6 0-2.9-3.2-3.2-4.6-1.9Z" />,
  usage: <path d="M2.6 12.4V9M6.2 12.4V5.4M9.8 12.4V7.2M13.4 12.4V3.4" />,
  account: (
    <>
      <circle cx="8" cy="5.6" r="2.6" />
      <path d="M2.9 13.4c.8-2.4 2.7-3.6 5.1-3.6s4.3 1.2 5.1 3.6" />
    </>
  ),
  versions: <path d="M8 2.4v7.4M5.2 7.2 8 10l2.8-2.8M3 12.8h10" />,
  appearance: (
    <>
      <circle cx="8" cy="8" r="3.1" />
      <path d="M8 1.4v1.8M8 12.8v1.8M1.4 8h1.8M12.8 8h1.8M3.4 3.4l1.3 1.3M11.3 11.3l1.3 1.3M12.6 3.4l-1.3 1.3M4.7 11.3l-1.3 1.3" />
    </>
  ),
  advanced: <path d="M3 5h5.2M11.2 5H13M3 11h2.2M8.2 11H13M9.4 3.4v3.2M6.4 9.4v3.2" />,
};

// Grouped by the question each answers, in the order they get asked: what this
// turn does, where it runs, what it keeps, what this machine is. Fourteen flat
// rows made the list something to scan; four short ones make it something to
// aim at.
export const NAV: [string, [Section, string][]][] = [
  [
    "这一轮",
    [
      ["session", "会话"],
      ["model", "模型"],
      ["tools", "工具与权限"],
      ["hooks", "自动化"],
      ["ext", "扩展"],
    ],
  ],
  [
    "在哪里跑",
    [
      ["network", "网络"],
      ["remote", "远程"],
      ["storage", "存储"],
    ],
  ],
  [
    "它记得什么",
    [
      ["memory", "记忆"],
      ["usage", "用量"],
    ],
  ],
  [
    "这台机器",
    [
      ["account", "账号"],
      ["versions", "版本"],
      ["appearance", "外观"],
      ["advanced", "高级"],
    ],
  ],
];
