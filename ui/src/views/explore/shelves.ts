import type { ListResult } from "../../api";

export const shelfBranchID = { bug: "shelf:bug", chore: "shelf:chore", tweak: "shelf:tweak" } as const;

export function selectedShelfBranch(noun: string | undefined): string[] {
  if (noun === "bug" || noun === "chore" || noun === "tweak") return [shelfBranchID[noun]];
  return [];
}

export function expandableShelfIDs(bugs: ListResult | null, chores: ListResult | null, tweaks: ListResult | null): string[] {
  const ids: string[] = [];
  if ((bugs?.nodes.length ?? 0) > 0) ids.push(shelfBranchID.bug);
  if ((chores?.nodes.length ?? 0) > 0) ids.push(shelfBranchID.chore);
  if ((tweaks?.nodes.length ?? 0) > 0) ids.push(shelfBranchID.tweak);
  return ids;
}
