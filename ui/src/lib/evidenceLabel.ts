// evidenceLabel is the one place an evidence ref becomes the words a person
// reads on a link. The ref itself stays on the link's href and title; it never
// prints. A known criterion reads as its eval output, a known kind reads in
// plain words (command_output becomes "command output evidence"), and
// anything else reads as plain "evidence".

export function evidenceLabel(input: { ref: string; kind?: string; criterion?: string }): string {
  const criterion = input.criterion ?? "";
  if (criterion !== "") {
    return `eval ${criterion} output`;
  }
  const kind = (input.kind ?? "").replaceAll("_", " ").trim().toLowerCase();
  if (kind !== "") {
    return `${kind} evidence`;
  }
  return "evidence";
}
