# cuppogan-site

Public marketing site for www.cuppogan.com, built with Astro as a fully static
site (no server, no form that stores data). Interest goes to the official
CuppoGan WhatsApp line.

```bash
npm install
npm run dev      # http://localhost:4321
npm run build    # static output in dist/
```

## Content rules

- `src/legal/terminos.html` and `src/legal/privacidad.html` are the published
  Terms and Privacy Policy (v1.0, effective 11 November 2026), copied verbatim.
  Change them only with a new version number and effective date.
- The Terms (§3, §7) say no page on cuppogan.com informs amounts, rates or
  conditions of a cupo. Keep tiers, rates and terms out of the marketing copy.
- Brand assets come from the brandbook v2.0 the app uses
  (`frontend/public`, `frontend/src/theme.ts`).
