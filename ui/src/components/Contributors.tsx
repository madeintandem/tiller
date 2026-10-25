import { Fragment } from "react";
import type { ContributorInfo } from "../api";
import { PersonName } from "../people";

export default function Contributors({ people }: { people: readonly ContributorInfo[] }) {
  if (people.length === 0) return <span className="muted">None yet</span>;
  return <>{people.map((person, index) => (
    <Fragment key={person.id}>
      {index > 0 && ", "}
      <PersonName id={person.id} name={person.name} />
    </Fragment>
  ))}</>;
}
