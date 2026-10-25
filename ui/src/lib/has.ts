// has reports whether an optional string is present and non-empty, narrowing
// it for JSX.

export function has(value: string | undefined): value is string {
  return value !== undefined && value !== "";
}
