// What a window can do and a page cannot, behind one interface so nothing in
// the app learns which shell it is running in. Electron exposes a preload
// bridge; a browser tab has none, and answers for itself.

export type Shell = "electron" | "browser";

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
   *  tab never learns a path. */
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
  /** Whether this shell draws the agent's browser pages inside the window. */
  drawsBrowserViews(): boolean;
  /** Draw one of the agent's pages over rect, in on-screen coordinates, and
   *  hide every other; hideBrowserView puts them all away again. */
  showBrowserView(target: string, rect: ViewRect): void;
  hideBrowserView(): void;
  controlBrowserView(target: string, action: BrowserControl): void;
  /** Load what the person typed. false when the shell refused the address. */
  navigateBrowserView(target: string, address: string): Promise<boolean>;
}

export interface ViewRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export type BrowserControl = "back" | "forward" | "reload" | "stop";

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
  showBrowserView?(target: string, rect: ViewRect): Promise<void>;
  hideBrowserView?(): Promise<void>;
  controlBrowserView?(target: string, action: string): Promise<void>;
  navigateBrowserView?(target: string, address: string): Promise<boolean>;
}

const bridge = () => (window as unknown as { reasonixHost?: ElectronBridge }).reasonixHost;

// Electron reports the platform under Node's names; the page spells them the
// way Go does, and one spelling is what keeps a CSS selector honest.
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
  // A shell older than the verbs has no views to draw, and says so by lacking them.
  drawsBrowserViews() {
    return typeof this.api.showBrowserView === "function";
  }
  showBrowserView(target: string, rect: ViewRect) {
    void this.api.showBrowserView?.(target, rect);
  }
  hideBrowserView() {
    void this.api.hideBrowserView?.();
  }
  controlBrowserView(target: string, action: BrowserControl) {
    void this.api.controlBrowserView?.(target, action);
  }
  navigateBrowserView(target: string, address: string) {
    return this.api.navigateBrowserView?.(target, address) ?? Promise.resolve(false);
  }
}

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
  drawsBrowserViews() {
    return false;
  }
  showBrowserView() {}
  hideBrowserView() {}
  controlBrowserView() {}
  navigateBrowserView() {
    return Promise.resolve(false);
  }
}

function pick(): HostPort {
  const api = bridge();
  if (api) return new ElectronHost(api);
  return new BrowserHost();
}

let chosen: HostPort | null = null;

/** The shell under this page, decided once. */
export function host(): HostPort {
  chosen ??= pick();
  return chosen;
}
