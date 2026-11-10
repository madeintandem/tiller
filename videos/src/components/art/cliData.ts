// Every tiller command, grouped by where it sits in the loop, and every skill. The grouping
// and wording follow the "whole CLI" slide of the Tiller talk. test/art.test.ts checks this
// list against `tiller --help` and `tiller guide --help` of the pinned version.
export type CliGroup = { title: string; commands: [name: string, what: string][] };

export const CLI_COLUMNS: CliGroup[][] = [
  [
    {
      title: "Set up",
      commands: [
        ["init", "scaffold .tiller/"],
        ["join", "who you are, wire agent"],
        ["add", "a person or a node"],
        ["remove", "retire a person or draft"],
        ["configure", "set review grain"],
        ["migrate", "refresh this checkout"],
        ["upgrade", "update Tiller"],
      ],
    },
    {
      title: "Plan",
      commands: [
        ["approve", "approve a proposed plan"],
        ["spec", "mark a feature specced"],
        ["order", "intended sibling order"],
        ["link", "record a dependency"],
        ["unlink", "remove a link"],
        ["rename", "retitle a node"],
        ["return", "undo an approval"],
        ["cancel", "terminate a node"],
      ],
    },
  ],
  [
    {
      title: "Build",
      commands: [
        ["claim", "reserve work"],
        ["start", "begin; owner is fixed"],
        ["check", "run and record gates"],
        ["eval", "run acceptance criteria"],
        ["attest", "human sign-off"],
        ["submit", "send to review"],
        ["release", "give back a claim"],
        ["reassign", "hand a claim over"],
        ["transfer", "hand off started work"],
      ],
    },
    {
      title: "Review & done",
      commands: [
        ["resume", "back from review"],
        ["sync", "merges become done"],
        ["resolve", "tie a fix to its merge"],
        ["archive", "shelve a done epic"],
      ],
    },
  ],
  [
    {
      title: "See",
      commands: [
        ["status", "project or node state"],
        ["tree", "the work tree"],
        ["ls", "list nodes"],
        ["show", "one node in full"],
        ["next", "your ranked next moves"],
        ["queue", "every pending action"],
        ["guide", "skills and next moves"],
        ["search", "keyword search"],
        ["query", "walk relationships"],
        ["readout", "one-block summary"],
        ["serve", "local web console"],
        ["usage", "agent token usage"],
      ],
    },
  ],
  [
    {
      title: "Change",
      commands: [
        ["amend", "propose a plan change"],
        ["withdraw", "drop an amendment"],
        ["intake", "land outside feedback"],
        ["answer", "close a question"],
        ["drift", "report what moved"],
        ["grade", "set a bug's severity"],
      ],
    },
  ],
];

export const SKILLS = [
  "orient",
  "derive-baseline",
  "decompose",
  "specify",
  "design-brief",
  "execute",
  "code-review",
  "capture",
  "amend",
  "intake",
  "mend-drift",
] as const;

export const ALL_COMMANDS: string[] = CLI_COLUMNS.flat().flatMap((g) => g.commands.map(([name]) => name));
