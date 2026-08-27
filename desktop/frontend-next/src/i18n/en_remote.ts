// English for the remote-machine surfaces: the sidebar's host section and
// the settings page's host book. Split out for the reason en_settings and
// en_kernel were — one screen's worth of wording, read together.

export const EN_REMOTE: Record<string, string> = {
  // ── 远程主机 ─────────────────────────────────────────────────
  "远程": "Remote",
  "断了，第 {n} 次重连": "Dropped — reconnecting, attempt {n}",
  "断了，正在重连": "Dropped — reconnecting",
  "连上了，但有转发没挂上": "Connected, but a forward did not attach",
  "已断开": "Disconnected",
  "这台主机还没有设默认工作区": "This host has no default workspace set",
  // The bootstrap's steps. Looked up by key rather than written out at a call
  // site, so the scanner cannot see them — they are listed here by hand, and a
  // missing one shows up as Chinese in an English window rather than as a test
  // failure.
  "探测平台": "Detecting the platform",
  "安装 reasonix": "Installing reasonix",
  "等待其他连接": "Waiting for another client",
  "启动 serve": "Starting serve",
  "等它就绪": "Waiting for it to come up",
  "挂载端口转发": "Attaching the port forward",
  "接上已在运行的": "Reusing the one already running",

  // ── 远程：设置页 ─────────────────────────────────────────────
  "把另一台机器上的工作区接过来：内核跑在那边，窗口还是这个。开的面板和本地的并排坐着，所以每处都写清楚它在哪台机器上。":
    "Bring a workspace on another machine here: the kernel runs over there, the window stays this one. Its panes sit beside your local ones, so every one of them says which machine it is on.",
  "{n} 台在线": "{n} online",
  "还没有远程机器。加一台，它的工作区就和本地的并排出现在左栏。":
    "No remote machines yet. Add one and its workspaces appear in the sidebar beside your local ones.",
  "加一台机器": "Add a machine",
  "没设默认工作区": "No default workspace",
  "跟随 ssh_config": "Follows ssh_config",
  "{n} 条转发": "{n} forwards",
  "{n} 个面板": "{n} panes",
  "从列表移除「{name}」？远端什么都不会删。": "Remove \u201c{name}\u201d from the list? Nothing on the remote is deleted.",
  "~/.ssh/config 里还有": "Also in ~/.ssh/config",
  "按 ssh_config 里的设置加进来": "Add it with the settings from ssh_config",
  "空着的项去 ~/.ssh/config 里找": "Take anything left blank from ~/.ssh/config",
  "地址": "Address",
  "用户": "User",
  "默认工作区": "Default workspace",
  "这台机器上的项目": "Projects on this machine",
  "远端的项目目录，例如 /srv/training": "A project directory over there, e.g. /srv/training",
  "默认": "Default",
  "设为默认": "Make default",
  "把 {path} 设成默认": "Make {path} the default",
  "不再列出 {path}": "Stop listing {path}",
  "还有 {n} 个项目": "{n} more projects",
  "私钥文件": "Private key file",
  "私钥口令的环境变量名": "Env var holding the key passphrase",
  "已连上": "Connected",
  "重连中": "Reconnecting",
  "有转发没挂上": "A forward did not attach",

  // ── 连接时的两个问题 ─────────────────────────────────────────
  "第一次连 {host}": "First connection to {host}",
  "这台机器还没见过。下面是它出示的指纹 —— 跟你从别处拿到的那份对一下，一致才接受。":
    "This machine has not been seen before. Below is the fingerprint it presented \u2014 compare it with the one you were given elsewhere, and accept only if they match.",
  "算法": "Algorithm",
  "指纹": "Fingerprint",
  "对得上，记住它": "They match, remember it",
  "私钥被口令锁着": "The private key is passphrase-locked",
  "{host} 要密码": "{host} wants a password",
  "解开 {file}。它只存在这次连接的内存里，不会写进任何文件。":
    "To unlock {file}. It lives in memory for this connection only and is never written to any file.",
  "只存在这次连接的内存里，不会写进任何文件。想让它记住，在设置里填一个环境变量名。":
    "It lives in memory for this connection only and is never written to any file. To have it remembered, name an environment variable in settings.",
  "继续": "Continue",
};
