// The Report page's Agent use tab. It fetches /api/report/usage itself, so the
// endpoint is read only when this tab renders. Above the figures a note says
// what the usage covers, because capture began on a different day per harness.

import type { UsageReport } from "../../api";
import { usageReportPath } from "../../api";
import { useFetch } from "../../hooks";
import ChartCard from "./ChartCard";
import FeatureUsageChart from "./FeatureUsageChart";
import ItemUsageChart from "./ItemUsageChart";
import { formatPercent, formatTokens, formatWeek } from "./format";
import { Tile } from "./KpiStrip";
import { ModelStageChart, StageChart } from "./StageCharts";
import { CacheReadsChart, FreshTokensChart } from "./UsageWeekCharts";

export default function AgentUseTab() {
  const { data, error } = useFetch<UsageReport>(usageReportPath);
  if (error !== null) return <p className="error">{error}</p>;
  if (data === null) return <p className="muted" role="status">Loading…</p>;
  if (data.harnesses.length === 0) return <p className="muted">No agent usage recorded yet</p>;
  const { kpis } = data;
  const median = kpis.median_per_item;
  // The tied share stays visible when no median exists yet; it is only
  // unknown when nothing was measured at all.
  const tied = kpis.total > 0 ? `${Math.round(kpis.tied_share * 100)}% of tokens tied to a work item` : undefined;
  const items = `${kpis.items} ${kpis.items === 1 ? "item" : "items"}`;
  return (
    <>
      <p className="report-usage-note">
        Measured: {data.harnesses.map((h) => `${h.label} since ${formatWeek(h.since)}`).join(" · ")}
      </p>
      <dl className="report-kpis">
        <Tile label="Total tokens" value={formatTokens(kpis.total)} />
        <Tile label="Cache reads" value={formatTokens(kpis.cache_read)} sub={`${formatPercent(kpis.cache_read, kpis.total)} of all`} />
        <Tile label="Fresh input and cache writes" value={formatTokens(kpis.input + kpis.cache_write)} />
        <Tile label="Output" value={formatTokens(kpis.output)} />
        <Tile
          label="Median per work item"
          value={median === null ? "Not yet" : formatTokens(median)}
          muted={median === null}
          sub={median === null || tied === undefined ? tied : `${items} · ${tied}`}
        />
      </dl>
      <div className="report-charts">
        <ChartCard>
          <CacheReadsChart usage={data} />
        </ChartCard>
        <ChartCard>
          <FreshTokensChart usage={data} />
        </ChartCard>
        <ChartCard>
          <StageChart usage={data} />
        </ChartCard>
        <ChartCard>
          <ModelStageChart usage={data} />
        </ChartCard>
        <div className="report-wide">
          <ChartCard>
            <FeatureUsageChart usage={data} />
          </ChartCard>
        </div>
        <div className="report-wide">
          <ChartCard>
            <ItemUsageChart usage={data} />
          </ChartCard>
        </div>
      </div>
    </>
  );
}
