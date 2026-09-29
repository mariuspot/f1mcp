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
