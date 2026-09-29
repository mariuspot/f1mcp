import { anthropic } from '@ai-sdk/anthropic';
import { generateText, type UIMessage } from 'ai';

// The models the chat can use. Auto sends lookups to Sonnet and analysis to
// Opus; the page can also pick one.
export const models = {
  sonnet: { id: 'claude-sonnet-5', label: 'Sonnet 5' },
  opus: { id: 'claude-opus-5-5', label: 'Opus 5.5' },
  fable: { id: 'claude-fable-5-1', label: 'Fable 5.1' },
} as const;

export type ModelChoice = 'auto' | keyof typeof models;

const routerModel = 'claude-haiku-4-5';

// route picks the model for a question: Opus for analysis (why, how, what
// made the difference, strategy, comparisons), Sonnet for lookups. Anything
// unclear, or a failed classification, goes to Sonnet.
export async function route(messages: UIMessage[], signal?: AbortSignal): Promise<keyof typeof models> {
  const recent = messages
    .filter(m => m.role === 'user')
    .slice(-2)
    .map(m => m.parts.flatMap(p => (p.type === 'text' ? [p.text] : [])).join(' '))
    .join('\n---\n');
  try {
    const { text } = await generateText({
      model: anthropic(routerModel),
      instructions:
        'Classify the last Formula 1 question (after the final ---, if any; earlier text is context). ' +
        'Answer "lookup" if it asks for facts that tools return directly: results, standings, schedules, lap times, ' +
        'maps, replays, who won, what happened. Answer "analysis" if it asks to explain, compare in depth, find ' +
        'reasons, judge strategy or performance, or combine several sources into a conclusion. Answer with one word.',
      prompt: recent,
      maxOutputTokens: 5,
      abortSignal: signal,
    });
    return /analysis/i.test(text) ? 'opus' : 'sonnet';
  } catch {
    return 'sonnet';
  }
}

// instructions are the F1 agent's system prompt.
export function instructions(now = new Date()) {
  return `You are Pit Wall, a Formula 1 analyst in a chat app. Today is ${now.toISOString().slice(0, 10)} (UTC).

Answer from the tools, never from memory: results, times, standings, laps and incidents all come from them. If a tool has no data, say so plainly.

Coverage: schedules, results, standings and drivers from 1950; everything detailed (laps, sectors, telemetry, pit stops, tyres, weather, race control, incidents, maps, lap comparisons and replays) only from 2023.

How to pick arguments:
- Events are a year plus a round: a number, "last", "next", or a name such as "Monaco" or "Spa". Leave the year out for the current season.
- Sessions: race (default), qualifying, sprint, sprint_qualifying, fp1-fp3. For laps, telemetry, comparisons and replays you can also give a part of qualifying: q1, q2, q3 (sq1-sq3 for sprint qualifying).
- Laps: a number, or best, first, last (last timed lap that isn't an out-lap) or last_flying (last push lap). "Final run" or "last attempt" in qualifying usually means last_flying.
- To compare drivers' laps use compareLaps; to show laps being driven use lapReplay (animate for a GIF); to explain a red flag or safety car use incident; for a circuit or corner use track.

The page draws each tool's answer as a card: tables, maps, comparisons and replays appear on their own, so don't repeat the tables or describe the images. Write a short answer around them: the result, the key numbers, and what stands out. Give lap times as m:ss.sss and gaps in seconds (e.g. +0.182 s). Use drivers' names or three-letter codes. Keep it tight; offer a follow-up (e.g. a comparison or replay) when it would help.`;
}
