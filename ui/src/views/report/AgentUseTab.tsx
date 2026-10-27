// The Report page's Agent use tab. It fetches /api/report/usage itself, so the
// endpoint is read only when this tab renders.

import type { UsageReport } from "../../api";
import { usageReportPath } from "../../api";
import { useFetch } from "../../hooks";

export default function AgentUseTab() {
  const { data, error } = useFetch<UsageReport>(usageReportPath);
  if (error !== null) return <p className="error">{error}</p>;
  if (data === null) return <p className="muted" role="status">Loading…</p>;
  if (data.harnesses.length === 0) return <p className="muted">No agent usage recorded yet</p>;
  return null;
}
