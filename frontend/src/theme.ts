import { createTheme } from "@mui/material/styles";

/**
 * CuppoGan brand system (Brandbook v2.0, September 2026).
 *
 * Green is the central asset and graphite carries the financial credibility.
 * The brand idea is "as trustworthy as a sector institution, as easy as a
 * modern fintech", so the palette stays sober: green marks action and positive
 * state, never decoration.
 */
export const brand = {
  deepField: "#06673D",
  midGreen: "#267E37",
  growthGreen: "#398E33",
  graphite: "#2D3131",
  warmCream: "#F4F2EB",
  /** Signature gradient. Print, embroidery and small sizes use flat deepField. */
  gradient: "linear-gradient(90deg, #06673D 0%, #398E33 100%)",
} as const;

const theme = createTheme({
  palette: {
    primary: {
      main: brand.deepField,
      light: brand.growthGreen,
      dark: "#044B2C",
      contrastText: "#FFFFFF",
    },
    secondary: {
      main: brand.graphite,
      light: "#4A4F4F",
      dark: "#1A1D1D",
      contrastText: "#FFFFFF",
    },
    background: {
      default: brand.warmCream,
      paper: "#FFFFFF",
    },
    text: {
      primary: brand.graphite,
      secondary: "#5C6160",
    },
    error: {
      main: "#B23B2E",
    },
    warning: {
      main: "#C08A2E",
    },
    // Green reads as "action or positive state" per the visual language rules.
    success: {
      main: brand.midGreen,
    },
    info: {
      main: brand.growthGreen,
    },
    divider: "rgba(45, 49, 49, 0.12)",
  },
  typography: {
    // Inter is the brand face; Arial is the documented Office fallback.
    fontFamily: "Inter, Arial, sans-serif",
    h1: { fontWeight: 900, letterSpacing: "-0.02em" },
    h2: { fontWeight: 900, letterSpacing: "-0.02em" },
    h3: { fontWeight: 800, letterSpacing: "-0.01em" },
    h4: { fontWeight: 800, letterSpacing: "-0.01em" },
    h5: { fontWeight: 700 },
    h6: { fontWeight: 700 },
    subtitle1: { fontWeight: 500 },
    subtitle2: { fontWeight: 500 },
    button: { fontWeight: 600 },
  },
  shape: {
    borderRadius: 10,
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: "none",
          fontWeight: 600,
          borderRadius: 10,
        },
        containedPrimary: {
          // Flat green, matching the brandbook's primary action button.
          boxShadow: "none",
          "&:hover": { boxShadow: "none", backgroundColor: "#055532" },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 14,
          boxShadow: "0 1px 3px rgba(45, 49, 49, 0.08)",
          border: "1px solid rgba(45, 49, 49, 0.08)",
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: "#FFFFFF",
          color: brand.graphite,
          boxShadow: "0 1px 0 rgba(45, 49, 49, 0.10)",
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: { fontWeight: 600 },
      },
    },
  },
});

export default theme;
