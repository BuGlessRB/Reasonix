import type { VersionHub } from "./port";

// What the desktop shell binds onto the window. The browser build has none
// of it, so every call site asks before using one — the same page runs in
// both places and only one of them has a shell underneath.
export interface WailsBind {
  go?: {
    main?: {
      App?: {
        PickWorkspace?: () => Promise<string>;
        Versions?: () => Promise<VersionHub>;
        PinVersion?: (version: string) => Promise<void>;
        GoToVersion?: (version: string) => Promise<void>;
        SavePluginExport?: (name: string) => Promise<{ path: string; required: string[] }>;
        // Replies to a question the link layer is blocked on — a first-seen
        // host key, or a locked private key.
        AnswerRemote?: (id: string, ok: boolean, text: string) => Promise<void>;
      };
    };
  };
}
