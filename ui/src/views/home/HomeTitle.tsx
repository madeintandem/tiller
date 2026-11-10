// The project's name and its meta line, rendered above Home's tab rail so the
// page names itself on every tab. It is the page's one h1.

import type { ProjectSummary } from "../../api";
import { PersonName } from "../../people";

export default function HomeTitle({ summary }: { summary: ProjectSummary }) {
  return (
    <div className="home-id">
      <h1>{summary.project.name}</h1>
      <p className="meta muted">
        <span className="mono">{summary.project.key}</span> · {summary.project.kind}
        {summary.contributors.length > 0 && (
          <>
            {" "}
            · Contributors:{" "}
            {summary.contributors.map((o, i) => (
              <span key={o.id}>
                {i > 0 && ", "}
                <PersonName id={o.id} name={o.name} />
              </span>
            ))}
          </>
        )}
      </p>
    </div>
  );
}
