# NekoDL Web Dashboard

React + TypeScript + Vite + Tailwind CSS. See the root [README.md](../README.md) "Design" section for the dark/green theme rules and the "no native `alert()`/`confirm()`/`prompt()`" constraint (enforced by `.oxlintrc.json`'s `no-alert` rule).

```bash
npm install
npm run dev      # dev server
npm run build    # type-check + production build
npm run lint      # oxlint
```

This currently talks to nothing — the core API only has a `/health` endpoint so far (see [TODO.md](../TODO.md) Phase 1). `src/App.tsx` is a placeholder shell with a working `ToastProvider`/`Toast` and `Modal` component pair to build the real task UI on top of.
