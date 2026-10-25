// shortRev is the header's own shortening of a compact revision token: the
// store writes a blob pin as the full 40-character "git:<sha>" and the node
// page shows seven, the schema's own written form. Only a git token is
// shortened; a sha256 fingerprint has no sha to shorten and passes through,
// and an absent token reads as nothing rather than "git:".
import { describe, expect, it } from "vitest";
import { shortRev } from "./revision";

describe("shortRev", () => {
  it("shortens a full git blob token to seven hex characters, prefix dropped", () => {
    expect(shortRev("git:4bf7c96ac7346dba2c360308c4f37ff50dbdf227")).toBe("4bf7c96");
  });

  it("leaves an already-short git token alone, prefix dropped", () => {
    expect(shortRev("git:7458e30")).toBe("7458e30");
  });

  it("passes a non-git token through unchanged", () => {
    expect(shortRev("sha256:c8ccdf9aa8294581c86804a53dbcdf66")).toBe(
      "sha256:c8ccdf9aa8294581c86804a53dbcdf66",
    );
  });

  it("reads an absent token as empty", () => {
    expect(shortRev(undefined)).toBe("");
    expect(shortRev("")).toBe("");
  });
});
