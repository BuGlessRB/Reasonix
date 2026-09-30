import type { FeedbackCategory } from "../port/feedback";

export interface FeedbackDraft {
  category: FeedbackCategory;
  body: string;
  contact: string;
}

const EMPTY: FeedbackDraft = { category: "bug", body: "", contact: "" };

let held: FeedbackDraft = EMPTY;

export function heldDraft(): FeedbackDraft {
  return held;
}

export function holdDraft(next: FeedbackDraft): void {
  held = next;
}

export function dropDraft(): void {
  held = EMPTY;
}
