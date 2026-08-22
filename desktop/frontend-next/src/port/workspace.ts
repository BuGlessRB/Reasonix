// Projects on this machine and what has changed inside one.
export interface WorkspaceChange {
  path: string;
  oldPath?: string;
  // git porcelain XY, trimmed: "M", "A", "D", "R", "??".
  status: string;
}

export interface WorkspaceChanges {
  // False when the workspace is not a git repository — the caller falls back
  // rather than showing an empty list as if nothing had changed.
  repo: boolean;
  changes: WorkspaceChange[];
}

export interface WorkspaceEntry {
  path: string;
  name: string;
}

// GET /workspaces. canSwitch is the server's answer, not the client's guess:
// a server reachable over the network refuses to be repointed at all.
export interface WorkspaceInfo {
  current: string;
  canSwitch: boolean;
  canIsolate: boolean;
  recents: WorkspaceEntry[];
  isolated?: boolean;
}
