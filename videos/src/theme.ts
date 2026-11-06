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

export const video = { width: 1920, height: 1080, fps: 30 } as const;

export const theme = { colors, fonts, video };
