/** Test files that cannot share a worker's module registry.
 *
 *  `vi.mock` is the case: a module another file already imported unmocked stays
 *  unmocked, and the mock then silently does nothing — the test passes alone and
 *  fails in the suite. These run in a project of their own.
 *
 *  The list is checked against the sources by `isolated.test.ts`, so adding a
 *  `vi.mock` without adding the file here fails rather than going quiet. */
export const ISOLATED = [
  "src/i18n/format.test.ts",
  "src/ui/settings-deferred.interaction.test.tsx",
];
