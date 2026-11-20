import { defineConfig } from "astro/config";

// Public marketing site for www.cuppogan.com. Fully static: no server, no
// forms that store data. Interest goes to the official WhatsApp line.
export default defineConfig({
  site: "https://www.cuppogan.com",
  trailingSlash: "never",
  build: { format: "file" },
});
