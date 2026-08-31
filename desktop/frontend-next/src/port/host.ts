// What a window can do and a page cannot, behind one interface so nothing in
// the app learns which shell it is running in. Wails publishes bound methods on
// window.go; Electron exposes a preload bridge; a browser tab has neither, and
// answers for itself.

export type Shell = "wails" | "electron" | "browser";

export interface HostInfo {
  shell: Shell;
  /** "darwin" | "windows" | "linux", or "" where the page cannot know. */
  platform: string;
  /** The window has no native title bar, so the top row of the page is one.
   *  Answered by the shell: only it knows how its window was created. */
  titleBar: boolean;
}

export interface HostPort {
  describe(): Promise<HostInfo>;
  minimiseWindow(): void;
  toggleMaximiseWindow(): void;
  isWindowMaximised(): Promise<boolean>;
  closeWindow(): void;
  openExternal(url: string): void;
  /** Where dropped files live. Empty where the shell cannot say — a browser
   *  tab never learns a path, and Wails reports them on its own channel. */
  pathsForFiles(files: File[]): string[];
  /** Put text on disk where the user picks. null means this shell has no save
   *  surface at all; "" means they dismissed the dialog, which is an answer. */
  saveText(name: string, content: string): Promise<string | null>;
  saveBytes(name: string, bytes: Uint8Array): Promise<string | null>;
  /** Ask for a directory. null means this shell has no picker at all; "" means
   *  they dismissed it, the same two answers saveText gives. startIn is where
   *  to open: the shell owns the dialog, the kernel owns which workspace runs,
   *  so the page carries one to the other. */
  pickFolder(startIn: string): Promise<string | null>;
}

// The preload bridge. Verbs only: the origin the page was loaded from and the
// credential that opens it never cross it.
interface ElectronBridge {
  shell: "electron";
  platform: string;
  titleBar: boolean;
  minimiseWindow(): Promise<void>;
  toggleMaximiseWindow(): Promise<void>;
  isWindowMaximised(): Promise<boolean>;
  closeWindow(): Promise<void>;
  openExternal(url: string): Promise<void>;
  pathForFile(file: File): string;
  saveText(name: string, content: string): Promise<string>;
  saveBytes(name: string, bytes: Uint8Array): Promise<string>;
  pickFolder(startIn: string): Promise<string>;
}

interface WailsShell {
  runtime?: { Environment?: () => Promise<{ platform?: string }> };
  go?: {
    main?: {
      App?: {
        MinimiseWindow?: () => Promise<void>;
        ToggleMaximiseWindow?: () => Promise<void>;
        IsWindowMaximised?: () => Promise<boolean>;
        CloseWindow?: () => Promise<void>;
        OpenExternal?: (url: string) => Promise<void>;
        SaveText?: (name: string, content: string) => Promise<string>;
        PickWorkspace?: () => Promise<string>;
      };
    };
  };
}

const bridge = () => (window as unknown as { reasonixHost?: ElectronBridge }).reasonixHost;
const wails = () => (window as unknown as WailsShell).go?.main?.App;

// Electron reports the Go name for two of the three; the page has always spelled
// them the way Wails does, and one spelling is what keeps a CSS selector honest.
function normalise(platform: string): string {
  if (platform === "win32") return "windows";
  return platform;
}

class ElectronHost implements HostPort {
  constructor(private readonly api: ElectronBridge) {}
  describe() {
    return Promise.resolve({
      shell: "electron" as const,
      platform: normalise(this.api.platform),
      titleBar: this.api.titleBar,
    });
  }
  minimiseWindow() {
    void this.api.minimiseWindow();
  }
  toggleMaximiseWindow() {
    void this.api.toggleMaximiseWindow();
  }
  isWindowMaximised() {
    return this.api.isWindowMaximised().catch(() => false);
  }
  closeWindow() {
    void this.api.closeWindow();
  }
  openExternal(url: string) {
    void this.api.openExternal(url);
  }
  pathsForFiles(files: File[]) {
    // Resolved one at a time because that is the shape the platform offers;
    // a file the shell cannot place answers with "" and is dropped here.
    return files.map((f) => this.api.pathForFile(f)).filter(Boolean);
  }
  saveText(name: string, content: string) {
    return this.api.saveText(name, content);
  }
  saveBytes(name: string, bytes: Uint8Array) {
    return this.api.saveBytes(name, bytes);
  }
  pickFolder(startIn: string) {
    return this.api.pickFolder(startIn);
  }
}

class WailsHost implements HostPort {
  async describe(): Promise<HostInfo> {
    const env = await (window as unknown as WailsShell).runtime?.Environment?.().catch(() => undefined);
    return { shell: "wails", platform: normalise(env?.platform ?? ""), titleBar: true };
  }
  minimiseWindow() {
    void wails()?.MinimiseWindow?.();
  }
  toggleMaximiseWindow() {
    void wails()?.ToggleMaximiseWindow?.();
  }
  isWindowMaximised() {
    return wails()?.IsWindowMaximised?.().catch(() => false) ?? Promise.resolve(false);
  }
  closeWindow() {
    void wails()?.CloseWindow?.();
  }
  openExternal(url: string) {
    void wails()?.OpenExternal?.(url);
  }
  // The paths arrive on the shell's own drop channel instead, which is why
  // nothing here can answer for a file the page is holding.
  pathsForFiles() {
    return [];
  }
  async saveText(name: string, content: string) {
    return (await wails()?.SaveText?.(name, content)) ?? null;
  }
  // Packing and saving are one binding in this shell; the caller reaches it
  // rather than assembling the bytes itself.
  saveBytes() {
    return Promise.resolve(null);
  }
  // The Go side titles the panel and opens it on the running workspace, which
  // it reads from the kernel directly, so startIn is the page telling this
  // shell something it already knows.
  async pickFolder() {
    return (await wails()?.PickWorkspace?.()) ?? null;
  }
}

// A tab has no window of its own to drive: the chrome that would call these is
// not rendered there, and a link opens the way a link always has.
class BrowserHost implements HostPort {
  describe() {
    return Promise.resolve({ shell: "browser" as const, platform: "", titleBar: false });
  }
  minimiseWindow() {}
  toggleMaximiseWindow() {}
  isWindowMaximised() {
    return Promise.resolve(false);
  }
  closeWindow() {}
  openExternal(url: string) {
    window.open(url, "_blank", "noopener,noreferrer");
  }
  pathsForFiles() {
    return [];
  }
  saveText() {
    return Promise.resolve(null);
  }
  saveBytes() {
    return Promise.resolve(null);
  }
  pickFolder() {
    return Promise.resolve(null);
  }
}

function pick(): HostPort {
  const api = bridge();
  if (api) return new ElectronHost(api);
  if ((window as unknown as WailsShell).runtime?.Environment) return new WailsHost();
  return new BrowserHost();
}

let chosen: HostPort | null = null;

/** The shell under this page, decided once. */
export function host(): HostPort {
  chosen ??= pick();
  return chosen;
}
