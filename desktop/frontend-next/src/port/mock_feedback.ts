import { HttpError } from "./http_error";
import { MockBackup } from "./mock_backup";
import { FEEDBACK_CODE, type FeedbackEnv, type FeedbackItem, type FeedbackMine, type FeedbackReceipt, type FeedbackRequest } from "./feedback";

const LIMITS = { bodyBytes: 8192, nameChars: 40, contactChars: 120, images: 3, imageBytes: 2 << 20, uploadBytes: 10 << 20 };

const ENV = {
  version: "v2.24.0", commit: "7ee4bbb", surface: "studio", os: "windows", osVersion: "10.0.19045",
  arch: "amd64", locale: "en-US", channel: "stable", providerKind: "deepseek",
};

const DAY = 86_400_000;
const ago = (days: number) => new Date(Date.now() - days * DAY).toISOString();

// One row per status, so the list is never drawn from a fixture that skips one.
const SEEDED: FeedbackItem[] = [
  { receipt: "FB-7K3M-9QX2", category: "bug", titleSnippet: "Sidebar loses its selection after a resize", status: "fixed", issueNumber: 11350, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11350", resolvedVersion: "v2.21.0", createdAt: ago(6), updatedAt: ago(1) },
  { receipt: "FB-2H8P-4WD7", category: "idea", titleSnippet: "Export a session as markdown", status: "recorded", issueNumber: 11302, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11302", createdAt: ago(4), updatedAt: ago(3) },
  { receipt: "FB-5N1C-8RT3", category: "bug", titleSnippet: "Paste of a long log freezes the composer", status: "in_progress", issueNumber: 11377, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11377", createdAt: ago(3), updatedAt: ago(1) },
  { receipt: "FB-9B4D-1XM6", category: "question", titleSnippet: "Where do I change the default model?", status: "received", createdAt: ago(0.1), updatedAt: ago(0.1) },
  { receipt: "FB-3F6G-2KV9", category: "idea", titleSnippet: "Fixed in the next build: quieter update banner", status: "fixed", issueNumber: 11390, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11390", resolvedVersion: "next", createdAt: ago(8), updatedAt: ago(0.5) },
  { receipt: "FB-8Q2J-6PW4", category: "other", titleSnippet: "Please support a monorepo layout", status: "wontfix", issueNumber: 11288, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11288", createdAt: ago(12), updatedAt: ago(5) },
  { receipt: "FB-4T7V-3ZH1", category: "bug", titleSnippet: "Crash when opening settings on a small window", status: "duplicate", issueNumber: 11301, issueUrl: "https://github.com/esengine/DeepSeek-Reasonix/issues/11301", duplicateOf: 11299, createdAt: ago(15), updatedAt: ago(9) },
  { receipt: "FB-6M9R-5CE8", category: "question", titleSnippet: "How do I move my sessions to another disk?", status: "received", statusUnavailable: true, createdAt: ago(40), updatedAt: ago(40) },
];

// A tab's own sessionStorage picks the refusal the next call answers with, so a
// dev page can show every failure without a kernel that will produce it.
const FAULT = "rx-mock-feedback-fault";

function fault(): string {
  try {
    return sessionStorage.getItem(FAULT) ?? "";
  } catch {
    return "";
  }
}

const STATUS: Record<string, number> = {
  [FEEDBACK_CODE.tooLarge]: 413, [FEEDBACK_CODE.rateLimited]: 429, [FEEDBACK_CODE.disabled]: 503,
  [FEEDBACK_CODE.duplicate]: 409, [FEEDBACK_CODE.badToken]: 409, [FEEDBACK_CODE.offline]: 502, [FEEDBACK_CODE.unavailable]: 502, [FEEDBACK_CODE.internal]: 500, [FEEDBACK_CODE.busy]: 503, [FEEDBACK_CODE.imageMetadata]: 400,
};

export class MockFeedback extends MockBackup {
  private filed: FeedbackItem[] = [];
  private name = "";

  async feedbackEnv(): Promise<FeedbackEnv> {
    return { env: ENV, displayName: this.name, limits: LIMITS };
  }

  async sendFeedback(req: FeedbackRequest): Promise<FeedbackReceipt> {
    const code = fault();
    if (code === "feedback.invalid") {
      throw new HttpError(400, "invalid", { code, error: "invalid", params: { field: "body", reason: "too_long" } });
    }
    if (code && STATUS[code]) {
      throw new HttpError(STATUS[code], code, { code, error: code, params: code === FEEDBACK_CODE.rateLimited ? { retryAfterSeconds: 90 } : {} });
    }
    this.name = req.displayName;
    const receipt = "FB-" + (1000 + this.filed.length * 7).toString(36).toUpperCase().padStart(4, "K") + "-9QX2";
    const now = new Date().toISOString();
    this.filed = [{ receipt, category: req.category, titleSnippet: req.body.trim().slice(0, 80), status: "received", createdAt: now, updatedAt: now }, ...this.filed];
    return { receipt, status: "received", createdAt: now, redacted: /sk-[A-Za-z0-9]{8,}/.test(req.body) };
  }

  async myFeedback(): Promise<FeedbackMine> {
    const code = fault();
    if (code === "mine_error") throw new HttpError(502, "unavailable", { code: FEEDBACK_CODE.unavailable, error: "unavailable" });
    return { items: [...this.filed, ...SEEDED], offline: code === "mine_offline" };
  }

}
