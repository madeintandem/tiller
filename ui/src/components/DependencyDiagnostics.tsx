import type { DependencyDiagnostics as DiagnosticReport } from "../api";

export default function DependencyDiagnostics({ report }: { report?: DiagnosticReport }) {
  if (!report || (report.gaps.length === 0 && report.advisories.length === 0)) return null;
  return (
    <section aria-label="Dependency diagnostics">
      {report.gaps.length > 0 && (
        <div role="alert" className="error">
          <h2>Execution dependency blockers</h2>
          <ul>{report.gaps.map((gap) => <li key={gap.message}>{gap.message}</li>)}</ul>
        </div>
      )}
      {report.advisories.length > 0 && (
        <div className="muted">
          <h2>Epic grouping advisories (nonblocking)</h2>
          <ul>{report.advisories.map((advisory) => <li key={advisory.message}>{advisory.message}</li>)}</ul>
        </div>
      )}
    </section>
  );
}
