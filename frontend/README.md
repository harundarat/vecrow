# Vecrow frontend

The web app for Vecrow: the landing page, wallet sign-in, the dashboard, account
settings, and the escrow pages that both parties share.

Next.js 16 (App Router, Turbopack), React 19, Tailwind CSS 4 and shadcn/ui
(`base-vega` style), run with Bun.

## Run it

The API from [`backend/`](../backend) must be running first (see the root
[README](../README.md#backend)).

```sh
bun install
cp env.example .env   # set NEXT_PUBLIC_REOWN_PROJECT_ID
bun dev               # http://localhost:5173
```

| Variable | Meaning |
| --- | --- |
| `NEXT_PUBLIC_API_URL` | Base URL of the API, e.g. `http://localhost:8080/api/v1` |
| `NEXT_PUBLIC_NEAR_NETWORK` | `testnet` or `mainnet`: the network NEAR wallets connect to |
| `NEXT_PUBLIC_REOWN_PROJECT_ID` | Reown (WalletConnect) project ID for EVM and Solana wallets |

`src/env.ts` validates them when the app starts. The backend must list
`http://localhost:5173` in `CORS_ALLOWED_ORIGINS`, and its `AUTH_DOMAIN` and `APP_URL`
must match the address the app is served from.

## Checks

```sh
bun run typecheck
bun run lint        # must end with 0 errors and 0 warnings
bun run build
bunx prettier --write <changed files>
```

There is no frontend test runner yet. Wallet encodings live in
`src/lib/wallet-encoding.ts` so they can be tested without a browser.

## Pages

| Route | What it does |
| --- | --- |
| `/` | Landing page. Its button opens the app when you are signed in. |
| `/sign-in` | Sign in with a NEAR, EVM or Solana wallet. The first sign-in creates the account. |
| `/sign-in/email` | Recovery sign-in with a code sent to the verified email. It can only link and unlink wallets. |
| `/app` | Escrows you created, joined or were invited to, with the next step for each. |
| `/app/escrows/new` | Create an escrow as the freelancer or the client and get its link. |
| `/app/settings` | Username, email, and linked wallets. |
| `/escrows/[id]` | One escrow: join, review, approve, pay in, submit, release, refund, cancel, and payout history. Works without an account through a guest wallet. |

## Code layout

| Path | Contents |
| --- | --- |
| `src/app/<route>/_components/` | Components used only by that route |
| `src/components/` | Shared components (`ui/` holds the shadcn primitives) |
| `src/lib/api/` | Backend calls and their types, mirroring the API's JSON |
| `src/lib/` | Pure helpers: USDC math, wallet encodings, escrow rules |
| `src/hooks/` | React hooks (wallet signer, escrow detail, approver and freelancer actions) |
| `src/context/` | Providers: auth session, NEAR wallets, Reown AppKit |

Shared building blocks for new pages: `PageHeader`, `FormSection`, `EmptyState`,
`LoadingState`, `FeedbackMessage` / `Notice`, `Address`, `CopyButton` / `CopyField` and
the status badges in `escrow-status-badge.tsx`.

## Conventions

The rules are in [`CLAUDE.md`](./CLAUDE.md). In short:

- USDC amounts are strings in base units end to end; math uses `BigInt`.
- Messages that a wallet signs come from the backend and are signed exactly as returned.
- A recovery session (opened with an email code) hides every action except wallet
  linking.
- The app has one dark theme: the light and dark tokens in `globals.css` are the same.

## Dependencies worth knowing

- `@x402/core`, `@x402/evm` and `@x402/svm` are not imported by our code. They are optional
  peer dependencies of `@coinbase/cdp-sdk`, which Reown's EVM adapter pulls in, and
  Turbopack fails to resolve its dynamic imports without them.
