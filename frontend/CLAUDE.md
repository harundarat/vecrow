@AGENTS.md

# Frontend conventions

## Tooling

- Use Bun: `bun install`, `bun run dev` (port 5173), `bun run typecheck`, `bun run lint`, `bun run build`.
- Format only the files you changed: `bunx prettier --write <files>`. Never run `bun run format` on the whole tree.
- No new state-management or data-fetching libraries. Use React state, context and the API client.

## UI

- Primitives come from shadcn (style `base-vega`): add them with `bunx shadcn add <name>`. Do not hand-write or copy a primitive that shadcn provides.
- Colors: only the theme tokens in `src/app/globals.css` (`bg-card`, `text-muted-foreground`, `text-destructive`, `neptune-*`, …). No new hex or ad-hoc color values.
- Buttons: use `Button` with its existing variants and sizes. Do not add variants.
- Class names: `cn` from `@/utils`.
- Every page must work at 375px wide and on desktop.

## Layout

- `src/app/<route>/_components/`: components used only by that route, next to it.
- `src/components/`: components shared across routes (`ui/` is shadcn's).
- `src/lib/api/`: backend calls and their types. `src/lib/`: other pure helpers (no React).
- `src/hooks/`: React hooks. `src/context/`: providers.
- `src/env.ts`: every environment variable, validated. Add new ones there and in `env.example`.

## Backend data

- Types mirror the backend JSON exactly, in snake_case (`expires_at`, `new_account`). Do not rename fields.
- USDC amounts are always strings, end to end. Never parse them into a JS number or float.
- Call the backend through `src/lib/api`. Authenticated calls go through `useAuth().request`, which clears the session on a 401.
- Signed messages always come from the backend and are signed exactly as returned. Never rebuild them in the frontend.

## Code

- Comments are in English and only explain why, when the code cannot show it. Do not reference docs/ IDs (R-xx, D-xx, §x.y).
- Wallet encodings (hex, base58, base64, NEP-413 nonce) live in `src/lib/wallet-encoding.ts`, so they can be tested without a browser.
