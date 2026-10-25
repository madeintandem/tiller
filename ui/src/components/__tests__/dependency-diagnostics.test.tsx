import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import DependencyDiagnostics from "../DependencyDiagnostics";

afterEach(cleanup);

it("keeps interleaving advisory evidence out of execution alerts while preserving simultaneous blockers", () => {
  const advisory = "office/later depends_on domain/work; features may interleave";
  const blocker = "a/first depends_on a/last; execution completion loop";
  const { rerender } = render(<DependencyDiagnostics report={{ ready: true, gaps: [], advisories: [{ reason: "projected_epic_order_cycle", message: advisory }] }} />);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: /nonblocking/ })).toBeInTheDocument();
  expect(screen.getByText(advisory)).toBeInTheDocument();

  rerender(<DependencyDiagnostics report={{ ready: false, gaps: [{ reason: "dependency_cycle", message: blocker }], advisories: [{ reason: "projected_epic_order_cycle", message: advisory }] }} />);
  const alert = screen.getByRole("alert");
  expect(within(alert).getByText(blocker)).toBeInTheDocument();
  expect(within(alert).queryByText(advisory)).not.toBeInTheDocument();
  expect(screen.getByText(advisory)).toBeInTheDocument();
});
