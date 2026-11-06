import { loadFont as loadDisplay } from "@remotion/google-fonts/SpaceGrotesk";
import { loadFont as loadBody } from "@remotion/google-fonts/Inter";
import { loadFont as loadMono } from "@remotion/google-fonts/SpaceMono";

// Loaded once at module load; Remotion delays rendering until the fonts are ready.
loadDisplay("normal", { weights: ["700"], subsets: ["latin"] });
loadBody("normal", { weights: ["400", "500"], subsets: ["latin"] });
loadMono("normal", { weights: ["400", "700"], subsets: ["latin"] });
