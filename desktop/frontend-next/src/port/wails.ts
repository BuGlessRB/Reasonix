// What the desktop shell binds onto the window. The browser build has none
// of it, so every call site asks before using one — the same page runs in
// both places and only one of them has a shell underneath.
export interface WailsBind {
  go?: {
    main?: {
      App?: {
        SavePluginExport?: (name: string) => Promise<{ path: string; required: string[] }>;
      };
    };
  };
}
