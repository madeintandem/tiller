// What the Explore layout hands its reading pane through the outlet: the
// served project's key, which per-project browser storage is namespaced by
// (undefined until /api/status answers, or when it failed). A pane rendered
// outside the layout reads undefined and simply keeps no per-project state.

import { useOutletContext } from "react-router-dom";

export interface ExploreOutletContext {
  projectKey: string | undefined;
}

export function useExploreProjectKey(): string | undefined {
  const context = useOutletContext<ExploreOutletContext | null | undefined>();
  return context?.projectKey;
}
