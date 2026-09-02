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

  // ── 远程：挑一个目录 ─────────────────────────────────────────
  "在 {name} 上挑一个目录打开": "Pick a folder on {name} to open",
  "在 {host} 上选一个目录": "Pick a folder on {host}",
  "浏览…": "Browse…",
  "那台机器上的路径": "A path on that machine",
  "上一级": "Up one level",
  "转到": "Go",
  "子目录": "Subfolders",
  "正在连上去…": "Connecting…",
  "就用这里": "Use this folder",
  "这个目录下面没有子目录了 —— 它自己就可以是工作区":
    "Nothing below this folder — it can be the workspace itself",
  "这个目录太大，只列了前面一部分。要找的在后面的话，直接把路径打上去。":
    "This folder is too big to list whole, so only the first entries are here. If what you want is further down, type its path.",

  // ── 远程：设置页 ─────────────────────────────────────────────
  "接入另一台机器上的工作区：内核在远程运行，界面仍在本机。远程面板与本地面板并排显示，每处都会标明所在的机器。":
    "Connect a workspace on another machine: the kernel runs there, the window stays here. Remote panes sit alongside local ones, so each place states which machine it belongs to.",
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
  "{host} 上没有 reasonix，而这台机器设了不自动装。把安装方式改回「自动」，或者自己去那边装一个": "There is no reasonix on {host}, and this machine is set never to install one. Switch installing back to automatic, or install one over there yourself",
  "{host} 上跑不了 npm —— 多半是那台机器没有 Node.js。装一个，或者把安装方式改成「上传」": "npm will not run on {host} — most likely it has no Node.js. Install one, or switch installing to upload",
  "npm 装好了，可它装到了登录 shell 找不到的地方。去那边调 npm prefix，或者把安装方式改成「上传」": "npm installed it somewhere the login shell does not look. Fix the npm prefix over there, or switch installing to upload",
  "本机的 reasonix 跑不了 {host} 的平台，也没有对应的官方包可下。把安装方式改成「npm」": "This machine's reasonix cannot run on {host}'s platform, and no official build for it was available. Switch installing to npm",
  "{host} 上装不上 reasonix —— npm、上传、下载都试过了。先自己去那台机器上装一个，再回来连": "Nothing could install reasonix on {host} — npm, upload and download were all tried. Install one there by hand, then connect again",
  "装到 {host} 上的 reasonix 跑不起来。那个目录可能挂了 noexec，也可能传到一半断了": "The reasonix installed on {host} will not run. That directory may be mounted noexec, or the transfer was cut short",
  "{host} 上的 reasonix 起来了，却一直没报出端口。去那边看一眼 ~/.reasonix/remote 下的日志": "reasonix started on {host} but never reported a port. Look at the logs under ~/.reasonix/remote over there",
  "跳板机": "Jump host",
  "登录密码的环境变量名": "Env var with the password",
  "安装方式": "Installing",
  "第一次连接要在那台机器上装一个 reasonix": "A first connect installs a reasonix on that machine",
  "自动挑一种": "Pick one automatically",
  "用远端的 npm": "Use the remote's npm",
  "传本机这个过去": "Upload this machine's",
  "不装，我自己装好了": "Do not install — there is one there already",
  "模型凭据": "Model credentials",
  "远端会话用哪台机器上配置的 Provider 和 Key": "Which machine's providers and keys a remote session uses",
  "用本机的，经隧道过去": "This machine's, over the tunnel",
  "用那台机器自己配的": "That machine's own",
  "问着…": "Asking…",
  "机器": "Machine",
  "那边是 {v}，太旧了，连的时候会换掉": "It has {v} there, too old — connecting would replace it",
  "还没有": "none yet",
  "跑不了": "will not run",
  "传过去": "upload",
  "下载": "download",
  "{name} 这条路走不通": "the {name} route is closed",
  "能连上 —— 那边有内核，或者装得上一个": "Ready — there is a kernel there, or one can be installed",
  "连不上 —— 上面任意一条解决掉就行": "Not ready — clearing any one of the above is enough",
};
