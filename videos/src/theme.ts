export const colors = {
  darkIndigo: "#001129",
  mediumIndigo: "#0A2C49",
  gracefulOlive: "#6F7347",
  brashBurgundy: "#8D134D",
  sharpieBlack: "#191819",
  websiteWhite: "#FBFBF5",
  fogGrey: "#F2F2F3",
  frenchGrey: "#E3DCD9",
  parchment: "#FFF4E6",
  rebelliousRed: "#F06553",
  matureYellow: "#FDBE14",
  coldIndigo: "#96BCBC",
  growingGreen: "#80998C",
  forestGreen: "#2D3F23",
  livelyLavender: "#F0E4EB",
  ripenedRed: "#962F1E",
  originalOchre: "#D3AD44",
  blackRaspberry: "#341225",
} as const;

export const fonts = {
  display: "Space Grotesk",
  body: "Inter",
  mono: "Space Mono",
} as const;

// Who is acting, wherever the video shows it: terminal badges, chat messages, process stages.
// Each has its own word as well as its own color, so neither reads by color alone.
export const actors = {
  you: { label: "You", color: colors.matureYellow },
  agent: { label: "Agent", color: colors.coldIndigo },
} as const;

// Where the company lives, as shown on the title screen and the closing slide.
export const site = { url: "madeintandem.com", email: "hello@madeintandem.com" } as const;

export const video = { width: 1920, height: 1080, fps: 30 } as const;

// Wordmark placement. The logo SVG aspect is 34725:15625; the guide's clearspace is 1X,
// where X is half the logo height. Content and lower thirds sit at or above `clearTop`.
const wordmarkWidth = 200;
const wordmarkHeight = (wordmarkWidth * 15625) / 34725;
const wordmarkBottom = 56;
const clearspace = wordmarkHeight / 2;
export const wordmark = {
  width: wordmarkWidth,
  height: wordmarkHeight,
  left: 120,
  bottom: wordmarkBottom,
  clearspace,
  // Distance from the frame bottom above which other content may sit.
  clearTop: Math.ceil(wordmarkBottom + wordmarkHeight + clearspace),
} as const;

export const theme = { colors, fonts, video, wordmark };
