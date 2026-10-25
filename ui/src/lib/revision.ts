// shortRev shortens a compact revision token for the node header's own
// display: the store pins a spec or eval blob as the full 40-character
// "git:<sha>" and the header shows seven hex characters, the schema's own
// written form, with the full token left to a hover title. Only a git token
// has a sha to shorten; a sha256 fingerprint passes through whole, and an
// absent token reads as nothing rather than as a bare prefix. The prefix is
// dropped because the surrounding label ("spec", "at commit") already says
// what the token is, and "git:" repeated three times on one line is noise.
export function shortRev(token: string | undefined): string {
  if (token === undefined || token === "") {
    return "";
  }
  const prefix = "git:";
  if (!token.startsWith(prefix)) {
    return token;
  }
  return token.slice(prefix.length, prefix.length + 7);
}
