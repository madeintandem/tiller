// ROW_CAP is how many rows any capped list on the console shows before it
// offers "N more". One number and one idiom everywhere, so a reader learns the
// grammar once: DecisionCard set it for the desk's needs-you cards, and the
// recap view's burst rows and expanded entry lists follow it rather than
// picking a second number for the same gesture.
//
// It stays 5 on the wider recap page deliberately. 8 was tempting for the
// extra width and there is no measurement to justify it: the live Acme
// tree's largest group is 3 burst rows, so the cap does not even fire there.
// Raise it once, here, when a dry run produces a window that argues for it.
export const ROW_CAP = 5;
