// In flight's grouping: a feature's tasks read under one heading instead of
// repeating the feature's path on every row.

export interface FeatureGroup<T> {
  featureId: string | null;
  featureTitle: string | null;
  items: T[];
}

// groupByFeature puts items of the same feature into the first group seen for
// that feature, adjacent or not. Groups keep first-seen order and items keep
// input order. Items with no feature (shelf bugs,
// chores, tweaks) never merge: each is its own group at its own position, so
// they render in place and the column's order holds.
export function groupByFeature<T extends { feature_id?: string; feature_title?: string }>(
  items: T[],
): FeatureGroup<T>[] {
  const groups: FeatureGroup<T>[] = [];
  const byFeature = new Map<string, FeatureGroup<T>>();
  for (const item of items) {
    const featureId = item.feature_id !== undefined && item.feature_id !== "" ? item.feature_id : null;
    let group = featureId === null ? undefined : byFeature.get(featureId);
    if (group === undefined) {
      group = { featureId, featureTitle: null, items: [] };
      if (featureId !== null) {
        byFeature.set(featureId, group);
      }
      groups.push(group);
    }
    if (group.featureTitle === null && item.feature_title !== undefined && item.feature_title !== "") {
      group.featureTitle = item.feature_title;
    }
    group.items.push(item);
  }
  return groups;
}

// A task's key is its feature's key plus one segment, so the feature's key is
// everything before the last slash.
export function featureKeyOf(taskKey: string | undefined): string | undefined {
  if (taskKey === undefined) {
    return undefined;
  }
  const slash = taskKey.lastIndexOf("/");
  return slash > 0 ? taskKey.slice(0, slash) : undefined;
}

// Under a feature heading a task's key shortens to its own last segment.
export function taskKeyTail(taskKey: string): string {
  return taskKey.slice(taskKey.lastIndexOf("/") + 1);
}
