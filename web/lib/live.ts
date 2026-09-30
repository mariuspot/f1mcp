// Types of the live page's API, from the Go service's internal/live.

export type Flag = 'green' | 'yellow' | 'vsc' | 'sc' | 'red' | 'chequered';

export type LapTime = { lap: number; seconds: number; sectors: (number | null)[]; pit_out?: boolean; purple_sectors: boolean[] };

export type Car = {
  number: number;
  code: string;
  name: string;
  team: string;
  color?: string;
  position?: number;
  gap_to_leader?: number;
  interval?: number;
  gap_text?: string;
  lap: number;
  last_lap?: LapTime;
  best_lap?: LapTime;
  compound?: string;
  tyre_age: number;
  stops?: { lap: number; stop_seconds?: number; compound_after?: string }[];
  out?: string;
  penalties?: string[];
};

export type LiveEvent = { id: number; time: string; lap?: number; kind: string; priority: number; drivers?: string[]; text: string };

export type Snapshot = {
  time: string;
  session: string;
  lap: number;
  flag: Flag;
  weather?: { air_c: number; track_c: number; humidity_pct: number; wind_m_s: number; rain: boolean };
  cars: Car[];
  best_lap?: { driver: string; lap: number; seconds: number };
};

export type Status = { race?: string; title?: string; speed?: number; running: boolean; time?: string; error?: string; run: number };

export type LiveResponse = { status: Status; snapshot: Snapshot; events: LiveEvent[] };

export type Race = { id: string; title: string; laps: number };
