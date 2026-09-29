# Pit Wall

A Formula 1 chat app: ask about races, drivers and laps, and get answers with tables, track maps, lap comparisons and replays. It's built with Next.js and the [AI SDK](https://ai-sdk.dev), and gets its data from the f1mcp Go service in the repository root.

## How it works

- `app/api/chat/route.ts` streams answers from Claude with the tools in `lib/tools.ts`.
- Each tool calls the Go service's web API (`POST /api/tools/{name}`), which returns the data plus image links.
- The model sees the data. The page draws each answer as a card (`components/cards.tsx`), with images proxied through `/img/{id}`.
- With the model set to Auto, `lib/agent.ts` sends each question through a quick Claude Haiku check: lookups go to Sonnet 5, analysis to Opus 5.5. The model picker can choose one instead.

## Running it

Start the Go service over HTTP from the repository root:

```sh
go run ./cmd/f1mcp -transport http   # http://localhost:8080
```

Then, in `web/`, with your key in `.env.local` (see `.env.example`):

```sh
npm install
npm run dev                          # http://localhost:3000
```

Or run both in Docker from the repository root, with `ANTHROPIC_API_KEY` in `.env`:

```sh
docker compose up --build            # http://localhost:3000
```

## Hosting

`compose.prod.yaml` runs it on one server behind [Caddy](https://caddyserver.com), which gets an HTTPS certificate for the domain automatically. On the server, with the repository cloned and a `.env` next to `compose.yaml`:

```sh
ANTHROPIC_API_KEY=…
PITWALL_DOMAIN=f1.mariuspotgieter.me
PITWALL_PASSCODE=…            # shared with the people who may use it
# PITWALL_HOURLY_LIMIT=30     # questions per visitor per hour
# PITWALL_DAILY_LIMIT=300     # questions per day in all
```

```sh
docker compose -f compose.yaml -f compose.prod.yaml up -d --build
```

Everything but the sign-in page needs the passcode. Signing in sets a cookie for 90 days, and changing `PITWALL_PASSCODE` signs everyone out. Questions are limited per visitor and per day, so a leaked passcode can't run up a large bill. Setting a monthly spend limit in the Anthropic Console is still a good idea.
