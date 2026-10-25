import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { ContributorInfo } from "./api";

type PersonLabel = (id: string, name?: string) => string;
const fallbackName: PersonLabel = (id, name) => name?.trim() || id;
const PeopleContext = createContext<PersonLabel>(fallbackName);

// Reuse the app's project roster; person labels never fetch independently.
export function PeopleProvider({ contributors, children }: { contributors?: readonly ContributorInfo[]; children: ReactNode }) {
  const label = useMemo<PersonLabel>(() => {
    const names = new Map(contributors?.map((contributor) => [contributor.id, contributor.name.trim() || contributor.id]));
    return (id, name) => names.get(id) ?? fallbackName(id, name);
  }, [contributors]);
  return <PeopleContext.Provider value={label}>{children}</PeopleContext.Provider>;
}

export function usePersonName(): PersonLabel {
  return useContext(PeopleContext);
}

export function PersonName({ id, name }: { id: string; name?: string }) {
  const label = usePersonName();
  return <span title={id}>{label(id, name)}</span>;
}
