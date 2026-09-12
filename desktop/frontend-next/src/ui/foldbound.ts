// How a stored fold bound reads as an intent. The field holds one number and
// the number is not the intent: 0 and 160000 fold at the same place today and
// mean different things the moment the default moves, and a negative value is
// not a smaller threshold at all — it retires the economic bound and leaves the
// window share still firing. Nobody should have to type a minus sign to say
// "protect capacity only", and nobody reading one should have to guess that it
// still folds.
//
// Shared because the setting is offered in two places — the settings sheet and
// the context rail — and those are one intent, not two. Only the labels differ,
// because one of them is drawn in a 245px column.
export type FoldMode = "default" | "custom" | "capacity";

export const foldModeOf = (stored: number): FoldMode =>
  stored < 0 ? "capacity" : stored > 0 ? "custom" : "default";

// What each mode writes. Custom is absent: it is a choice before it is a
// number, and the write happens when a number is committed.
export const foldModeValue: Partial<Record<FoldMode, number>> = { default: 0, capacity: -1 };
