import { tool } from 'ai';
import { z } from 'zod';
import { callTool, type ToolOutput } from './f1api';

// The chat agent's tools. Each runs a tool of the Go service. The model sees
// the data; the images go only to the page, which draws each tool's answer
// as a card.

const year = z.number().int().optional().describe('season, e.g. 2024; defaults to the current season');
const round = z
  .string()
  .optional()
  .describe("round number, 'last' (default), 'next', or part of the event, circuit, city or country name, e.g. 'Monaco' or 'Spa'");
const session = z
  .string()
  .optional()
  .describe('race (default), qualifying, sprint, sprint_qualifying, fp1, fp2 or fp3');
const lapSession = z
  .string()
  .optional()
  .describe('race (default), qualifying, sprint, sprint_qualifying, fp1, fp2, fp3, or a part of qualifying: q1, q2, q3 (sq1-sq3 for sprint qualifying)');
const driver = z.string().describe('driver code (VER), car number or name');
const lapChoice = z
  .string()
  .optional()
  .describe("a lap number such as '17', or best (default), first, last (the last timed lap that isn't an out-lap) or last_flying (the last push lap)");
const page = {
  limit: z.number().int().optional().describe('items per page (default 200)'),
  cursor: z.string().optional().describe('next_cursor from the previous page'),
};

// f1Tool makes a tool that calls the Go tool goName with the model's input.
function f1Tool<INPUT extends Record<string, unknown>, RESULT>(
  goName: string,
  description: string,
  inputSchema: z.ZodType<INPUT>,
  toArgs: (input: INPUT) => Record<string, unknown> = input => input,
) {
  return tool({
    description,
    inputSchema,
    execute: async (input: INPUT, { abortSignal }): Promise<ToolOutput<RESULT>> =>
      callTool<RESULT>(goName, toArgs(input), abortSignal),
    // The model gets the data and what the user can see, not image links.
    toModelOutput: ({ output }) => ({
      type: 'json' as const,
      value: JSON.parse(
        JSON.stringify({
          result: output.result,
          images_shown_to_user: output.images?.map(i => i.name),
        }),
      ),
    }),
  });
}

export const tools = {
  schedule: f1Tool(
    'get_schedule',
    "A season's calendar: rounds, event names, circuits (with circuit IDs), session start times (UTC) and sprint weekends. Any season from 1950.",
    z.object({ year }),
  ),
  sessionResults: f1Tool(
    'get_session_results',
    "A session's classification: race and sprint results (grid, laps, status, time or gap, points), qualifying Q1-Q3 times, or practice best laps. Any season for races and qualifying; practice and sprint qualifying from 2023.",
    z.object({ year, round, session }),
  ),
  standings: f1Tool(
    'get_standings',
    "The drivers' or teams' championship after a round: position, points, wins.",
    z.object({
      year,
      round: z.string().optional().describe('standings after this round (number or name); defaults to the latest'),
      championship: z.enum(['drivers', 'teams']).optional().describe('drivers (default) or teams'),
    }),
  ),
  drivers: f1Tool(
    'list_drivers',
    "Drivers in a season or event: name, code, number, team, nationality; from 2023 also team colour and headshot.",
    z.object({ year, round: z.string().optional().describe('only drivers entered in this round; default the whole season') }),
  ),
  laps: f1Tool(
    'get_laps',
    "Lap and sector times (seconds) for a session or part of qualifying, from 2023. By default each driver's best lap, fastest first, with the gap; with driver, every lap of that driver; with lap, that lap for everyone.",
    z.object({ year, round, session: lapSession, driver: driver.optional(), lap: z.number().int().optional(), ...page }),
  ),
  raceOrder: f1Tool(
    'get_race_order',
    'Running order of a race or sprint at the end of a lap (default the last), from 2023: positions, gap to leader, interval, laps down; with lap_chart, every position on every lap.',
    z.object({ year, round, session, lap: z.number().int().optional(), lap_chart: z.boolean().optional() }),
  ),
  pitStops: f1Tool(
    'get_pit_stops',
    "A race's pit stops, from 2023: lap, pit lane and stationary time in seconds, tyres before and after.",
    z.object({ year, round, driver: driver.optional() }),
  ),
  tyreStints: f1Tool(
    'get_tyre_stints',
    "Each driver's tyre stints in a session, from 2023: compound, first and last lap, tyre age at the start.",
    z.object({ year, round, session, driver: driver.optional() }),
  ),
  raceControl: f1Tool(
    'get_race_control',
    'Race control messages for a session, from 2023: by default key events (red flags, safety cars, penalties, investigations); with all, every message including yellow flags by marshal sector.',
    z.object({
      year,
      round,
      session,
      all: z.boolean().optional(),
      category: z.string().optional().describe('Flag, SafetyCar, Drs, CarEvent or Other'),
      from_lap: z.number().int().optional(),
      to_lap: z.number().int().optional(),
      ...page,
    }),
  ),
  weather: f1Tool(
    'get_weather',
    'Weather during a session, from 2023: air and track temperature, humidity, wind, rain; with detail, the readings.',
    z.object({ year, round, session, detail: z.boolean().optional(), ...page }),
  ),
  carTelemetry: f1Tool(
    'get_car_telemetry',
    "Car telemetry for one driver's lap, from 2023: top and minimum speed, full throttle and braking share, gear changes, and each braking zone with the corner. Needs a lap number (find it with laps).",
    z.object({ year, round, session: lapSession, driver, lap: z.number().int(), detail: z.boolean().optional(), ...page }),
  ),
  track: f1Tool(
    'get_track_map',
    "A circuit's map and layout (circuits raced from 2023): length, turns with names and elevation, straights, sectors. With corner, a close-up of that turn. Find it by circuit, or by year and round.",
    z.object({
      circuit: z.string().optional().describe("circuit ID (e.g. 'spa') or name, city or country"),
      year: z.number().int().optional().describe("season; picks that year's layout"),
      round: z.string().optional().describe('the circuit of this round of the year'),
      corner: z.number().int().optional().describe('draw a close-up of this turn'),
    }),
  ),
  incident: f1Tool(
    'get_incident',
    'Explain a red flag, safety car, virtual safety car or yellow flag, from 2023: which cars stopped or slowed and where, and the flagged sectors, drawn on the track map with a close-up of the nearest turn.',
    z.object({
      year,
      round,
      session,
      event: z.enum(['red', 'sc', 'vsc', 'yellow']).optional().describe('red (default), sc, vsc or yellow'),
      from_lap: z.number().int().optional().describe('the first such event on or after this lap'),
      drivers: z.array(z.string()).optional().describe('drivers known to be involved'),
    }),
  ),
  compareLaps: f1Tool(
    'get_lap_animation',
    "Compare 2-4 drivers' laps, from 2023: a map coloured by who was faster through each corner and straight, the gap all round the lap, each stretch's times, and each lap's time, tyre and top speed. Without laps, the two fastest in the session.",
    z.object({
      year,
      round,
      session: lapSession,
      laps: z
        .array(z.object({ driver, lap: lapChoice }))
        .max(4)
        .optional()
        .describe('the drivers to compare, each with their lap'),
    }),
    input => ({ ...input, format: 'faster' }),
  ),
  lapReplay: f1Tool(
    'get_lap_animation',
    "Replay 1-4 drivers' laps on the track map, from 2023, as if they started together: a still of the end of the lap, or with animate an animated GIF at 3x speed (slower to make). Shows speed, gear, throttle, brake, tyre and the gap.",
    z.object({
      year,
      round,
      session: lapSession,
      laps: z
        .array(z.object({ driver, lap: lapChoice }))
        .max(4)
        .optional()
        .describe('the drivers, each with their lap; default the fastest driver'),
      animate: z.boolean().optional().describe('an animated GIF instead of a still'),
      braking: z.boolean().optional().describe('also mark braking zones and gear changes'),
    }),
    ({ animate, ...input }) => ({ ...input, format: animate ? 'gif' : 'png' }),
  ),
};

export type F1Tools = typeof tools;
