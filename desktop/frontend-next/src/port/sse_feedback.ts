import { SseBackup } from "./sse_backup";
import type { FeedbackEnv, FeedbackMine, FeedbackReceipt, FeedbackRequest } from "./feedback";

export class SseFeedback extends SseBackup {
  feedbackEnv(locale?: string) {
    return this.get<FeedbackEnv>("/feedback/env" + (locale ? "?locale=" + encodeURIComponent(locale) : ""));
  }
  sendFeedback(req: FeedbackRequest) {
    return this.post0<FeedbackReceipt>("/feedback", req);
  }
  myFeedback() {
    return this.get<FeedbackMine>("/feedback/mine");
  }
}
