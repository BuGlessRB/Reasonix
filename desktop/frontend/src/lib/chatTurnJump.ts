import type { ChatMountedOrder } from "./chatMountedOrder";
import type { ChatScrollController } from "./chatScrollController";
import type { TranscriptOutlineEntry } from "./transcriptProtocol";

export type TurnJumpStatus = "idle" | "loading" | "failed";

export interface TurnJumpState {
  /** Stable identity of the turn being located, or null when idle. */
  readonly turn: string | null;
  readonly status: TurnJumpStatus;
  readonly error?: string;
}

const IDLE: TurnJumpState = Object.freeze({ turn: null, status: "idle" });

/** Frames to let the progressive mount advance before re-checking the target. */
const MOUNT_SETTLE_FRAMES = 120;
/** Pages a single jump may pull before giving up, independent of entry count. */
const MAX_JUMP_PAGES = 400;

export interface TurnJumpDeps {
  readonly mounts: ChatMountedOrder;
  readonly scroll: ChatScrollController;
  /** One older body page. Resolves false when the page added nothing usable. */
  loadOlder: () => Promise<boolean>;
  hasOlder: () => boolean;
  /** The DOM key of a turn once its node is mounted. */
  resolveKey: (entry: TranscriptOutlineEntry) => string | undefined;
  /** False once the session, tab, or snapshot this jump belongs to is gone. */
  isCurrent: () => boolean;
}

/**
 * Loads history until an unloaded turn's node is really mounted, then hands the
 * scroll write to the shared gateway. It never assumes "the data arrived" means
 * "the DOM exists": each page commits, the progressive mount advances, and the
 * target is re-resolved before the viewport moves.
 *
 * One transaction at a time. Reader intent, an explicit cancel, a newer target,
 * or a session/snapshot replacement all end the pending transaction; a page
 * already in flight may finish, but it can never take scroll control back.
 */
export class ChatTurnJump {
  private listeners = new Set<() => void>();
  private state: TurnJumpState = IDLE;
  /** Interaction id; a newer target or a cancel invalidates the pending loop. */
  private interaction = 0;
  private unsubscribeReader: (() => void) | undefined;

  constructor(private readonly deps: TurnJumpDeps) {}

  getSnapshot = (): TurnJumpState => this.state;
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };

  /** Reader intent observed on the transcript ends any pending jump. */
  private watchReader(): void {
    this.unsubscribeReader ??= this.deps.scroll.subscribeReaderIntent(() => { this.cancel(); });
  }

  cancel(): void {
    if (this.state.status === "idle") return;
    this.interaction++;
    this.unsubscribeReader?.();
    this.unsubscribeReader = undefined;
    this.publish(IDLE);
  }

  dispose(): void {
    this.cancel();
    this.listeners.clear();
  }

  async jump(entry: TranscriptOutlineEntry): Promise<void> {
    const interaction = ++this.interaction;
    const current = () => this.interaction === interaction && this.deps.isCurrent();
    // Exit follow first: the reader asked for a specific turn, and a tail pin
    // would otherwise fight the write that lands later.
    this.deps.scroll.stopFollowing();
    this.unsubscribeReader?.();
    this.unsubscribeReader = undefined;
    this.watchReader();
    this.publish({ turn: entry.id, status: "loading" });

    let pages = 0;
    try {
      for (;;) {
        if (!current()) return;
        const mounted = this.deps.resolveKey(entry);
        if (mounted !== undefined) {
          // The node exists in the document; the shared writer owns the move.
          if (!current()) return;
          this.deps.scroll.jump(mounted);
          this.finish(entry, interaction);
          return;
        }
        if (!this.deps.hasOlder()) {
          this.fail(entry, interaction, "turnUnavailable");
          return;
        }
        if (pages >= MAX_JUMP_PAGES) {
          this.fail(entry, interaction, "turnUnavailable");
          return;
        }
        pages++;
        const before = this.deps.mounts.getSnapshot();
        const loaded = await this.deps.loadOlder();
        if (!current()) return;
        if (!loaded) {
          this.fail(entry, interaction, "turnUnavailable");
          return;
        }
        await this.settleMounts(before);
        if (!current()) return;
      }
    } catch (error) {
      this.fail(entry, interaction, error instanceof Error ? error.message : "turnUnavailable");
    }
  }

  /**
   * Wait for the batched mount to advance, bounded so a page that adds no new
   * turn cannot stall the loop or spin the network.
   */
  private settleMounts(previous: readonly string[]): Promise<void> {
    // A page can already have advanced the mount while it was loading; the
    // published reference is what changes, so compare against it rather than
    // waiting for a publication that may never come.
    if (this.deps.mounts.getSnapshot() !== previous) return Promise.resolve();
    return new Promise((resolve) => {
      let elapsed = 0;
      let handle = 0;
      const step = (): void => {
        if (this.deps.mounts.getSnapshot() !== previous || elapsed >= MOUNT_SETTLE_FRAMES) { resolve(); return; }
        elapsed++;
        handle = requestAnimationFrame(step);
      };
      handle = requestAnimationFrame(step);
      void handle;
    });
  }

  private finish(entry: TranscriptOutlineEntry, interaction: number): void {
    if (this.interaction !== interaction) return;
    this.unsubscribeReader?.();
    this.unsubscribeReader = undefined;
    this.publish({ turn: entry.id, status: "idle" });
  }

  private fail(entry: TranscriptOutlineEntry, interaction: number, error: string): void {
    if (this.interaction !== interaction) return;
    this.unsubscribeReader?.();
    this.unsubscribeReader = undefined;
    this.publish({ turn: entry.id, status: "failed", error });
  }

  private publish(state: TurnJumpState): void {
    this.state = state;
    for (const listener of [...this.listeners]) listener();
  }
}
