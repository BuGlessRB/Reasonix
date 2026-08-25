// English for the storage panel: what is on disk, where it lives, and moving
// it. Split out of en.ts because that file is one screen's worth of catalogue
// per section and had grown past its ceiling — the grouping is the same.
export const EN_STORAGE: Record<string, string> = {
  存储: "Storage",
  "它把数据写在哪、占了多少。会话和索引会一直长，配置和凭据不会 —— 所以只有前者能搬走，搬迁在重启后生效。":
    "Where it writes, and how much. Sessions and indexes grow without limit, configuration and credentials do not — so only the first kind can move, and a move takes effect on the next launch.",
  "读不到存储占用。": "Cannot read what is on disk.",
  "正在统计…": "Measuring…",
  占用: "Space used",
  上一个位置还留着东西: "Still in the previous location",
  "这些还在 {dir}：{names}。移动存储位置时它们没有被一起带走，所以这台机器上的壁纸、主题包或更新回滚备份可能看起来不见了。手动把这几个目录复制到当前位置即可恢复。":
    "These are still in {dir}: {names}. A move left them behind, so a wallpaper, a theme pack or the backups an update rolls back to can look gone on this machine. Copying those folders into the current location restores them.",
  位置: "Locations",
  搬迁: "Move",
  "搬迁没能开始。": "The move could not be started.",
  "{drive} 剩余 {free} / {total}": "{drive} — {free} free of {total}",
  "由 {env} 指定": "set by {env}",
  不可移动: "cannot be moved",
  "移动…": "Move…",
  开始搬迁: "Start the move",
  指向这里: "Point it here",
  "目标文件夹的完整路径，空文件夹，或本来就存着这块数据的那个":
    "Full path to the destination folder — an empty one, or the one that already holds this data",
  "将搬走 {size}（{n} 个文件），目标盘剩余 {free}。完成后需要重启才会生效。":
    "Moves {size} in {n} files; the destination has {free} free. Takes effect after a restart.",
  "这个文件夹里已经存着这块数据（{size}，{n} 个文件）。直接指过去就行，不复制、也不删。重启后生效。":
    "This folder already holds that data ({size} in {n} files). It will simply be pointed at — nothing is copied and nothing is deleted. Takes effect after a restart.",
  "当前位置还留着 {size}，不会一起带过去。":
    "The current location still holds {size}, which is not carried across.",
  "已搬完。重启后生效。": "Moved. It takes effect after a restart.",
  正在指向新位置: "Pointing at the new location",
  正在复制: "Copying",
  正在校验: "Verifying",
  "已提交，正在清理原位置": "Committed; clearing the old location",
  会话与归档: "Sessions and archives",
  "转录、压缩归档、用量统计、回溯快照":
    "Transcripts, compaction archives, usage stats, rewind snapshots",
  索引与缓存: "Indexes and caches",
  "搜索索引与派生数据，删掉会自动重建":
    "Search indexes and derived data; deleting them rebuilds them",
  隔离工作区: "Isolated worktrees",
  交付模式检出的独立副本: "The separate checkouts Delivery works in",
  配置与凭据: "Configuration and credentials",
  "设置和 API key，始终随用户配置文件走":
    "Settings and API keys; these always stay with your user profile",
  进程锁: "Process locks",
  "多个实例互斥用，必须留在本机固定位置":
    "How instances exclude each other; these must stay in one place on this machine",
};
