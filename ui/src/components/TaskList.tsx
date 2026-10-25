// TaskList renders a feature's tasks in exactly the order the payload carries:
// the sibling dependency order gen.OrderTasks ranks them by (artifact-schema.md,
// Tasks: Ordering), never re-sorted here, so the console and `tiller tree` show
// one sequence.
//
// A task is a folder node with a key and a page of its own now, so the row
// follows the console's reference carve-out unchanged (ui-surface.md, "Rendered
// rows carve out their own reference convention"): the title leads, the key
// follows as muted mono at the margin, and the id never appears on a row. What
// trails is the LOCAL key segment rather than the canonical three, because the
// two segments above it are the feature this list already sits on; the full
// reference is one click away on the task's own page.
//
// The link still addresses by id, the whole console's rule (RefLink's own doc
// comment): a choice now, not a route constraint, since the /node/:noun/*
// splat route resolves a key just as well and the API resolves either.
//
// Each row renders its own raw wire status through Chip, so a mixed-status
// list labels every task instead of asking the header rollup to carry row-level
// meaning. Omitting children is deliberate: Chip falls back to the exact
// status word while its total status mapping supplies the color.

import { Link } from "react-router-dom";
import type { RelatedNode } from "../api";
import { composeStat } from "../views/home/stat";
import InstrSection from "./InstrSection";
import Chip from "./Chip";

// rowLabel is the task's own prose handle, the title the reference carve-out
// calls for ("rendered console rows show title first"): a dangling edge (an
// empty noun/id, the same degradation RefLink handles) falls back to the
// row's precomputed ref rather than a blank link.
function rowLabel(task: RelatedNode): string {
  return task.title !== undefined && task.title !== "" ? task.title : task.ref;
}

// localKey is the last segment of a canonical key, which is the task's own
// folder name (artifact-schema.md, Keys and paths). A task the tree could not
// place carries no key and so trails nothing; the row still renders.
function localKey(task: RelatedNode): string {
  if (task.key === undefined || task.key === "") {
    return "";
  }
  const i = task.key.lastIndexOf("/");
  return i >= 0 ? task.key.slice(i + 1) : task.key;
}

export default function TaskList({ tasks }: { tasks: RelatedNode[] }) {
  const { caption } = composeStat(tasks);
  // A feature with no tasks yet draws no list. "Tasks · 0" over "(no tasks
  // yet)" was two lines to report nothing, on the state most features in a
  // young tree are in; the feature's own status already says where it is in
  // its lifecycle.
  if (tasks.length === 0) {
    return null;
  }
  return (
    <InstrSection label={`Tasks · ${tasks.length} · dependency order`} caption={caption}>
      {tasks.map((task) => (
        <div className="instr-taskrow" key={task.id}>
          <Chip status={task.status} />
          <div className="instr-taskrow-body">
            {task.noun === "" || task.id === "" ? (
              <span>{rowLabel(task)}</span>
            ) : (
              <Link to={`/node/${task.noun}/${task.id}`}>{rowLabel(task)}</Link>
            )}
          </div>
          <span className="mono muted instr-taskrow-key">{localKey(task)}</span>
        </div>
      ))}
    </InstrSection>
  );
}
