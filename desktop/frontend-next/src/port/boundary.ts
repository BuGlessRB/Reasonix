// boundary.ts — the tool boundary as an editor sees it: which calls are
// refused before they run, and how far an approved one may write.

// The fine-grained gate. The three lists are checked in the order deny → ask →
// allow, and mode decides only what nothing matched. deny is the one entry no
// approval prompt can talk its way past, which is why it is worth a screen.
export interface PermissionLists {
  mode: string;
  allow: string[];
  ask: string[];
  deny: string[];
}

// What an editor needs beyond the lists: the file a save lands in, and — only
// when a project config declares its own — the merge actually in force, which
// an edit here cannot move.
export interface PermissionRules extends PermissionLists {
  path: string;
  shadowedBy?: string;
  effective?: PermissionLists;
}

// Where an approved write may land, and whether bash runs jailed.
// effectiveWriteRoots is the expansion the confiner will use: an empty
// workspaceRoot is not "anywhere", it is "the session directory".
export interface SandboxSettings {
  bash: string;
  network: boolean;
  workspaceRoot: string;
  allowWrite: string[];
  effectiveWriteRoots: string[];
  // False where this host has no OS sandbox at all — enforce would then refuse
  // every bash call rather than run it unconfined, so the switch says so
  // instead of pretending to work.
  available: boolean;
  why?: string;
  platform: string;
  path: string;
  shadowedBy?: string;
}
