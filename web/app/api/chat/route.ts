import { anthropic } from '@ai-sdk/anthropic';
import {
  convertToModelMessages,
  createUIMessageStreamResponse,
  isStepCount,
  streamText,
  toUIMessageStream,
} from 'ai';
import { instructions, models, route, type ModelChoice } from '@/lib/agent';
import { allowQuestion } from '@/lib/limits';
import { tools } from '@/lib/tools';
import type { ChatMessage } from '@/lib/types';

// Tool calls to OpenF1 can take a while; allow for a few in one answer.
export const maxDuration = 300;

export async function POST(req: Request) {
  const limited = allowQuestion(req);
  if (limited) return limited;

  const { messages, model = 'auto' }: { messages: ChatMessage[]; model?: ModelChoice } = await req.json();

  const routed = model === 'auto';
  const choice = routed ? await route(messages, req.signal) : model in models ? (model as keyof typeof models) : 'sonnet';

  const result = streamText({
    model: anthropic(models[choice].id),
    instructions: instructions(),
    messages: await convertToModelMessages(messages, { tools }),
    tools,
    stopWhen: isStepCount(12),
    abortSignal: req.signal,
  });

  return createUIMessageStreamResponse({
    stream: toUIMessageStream({
      stream: result.stream,
      originalMessages: messages,
      messageMetadata: ({ part }) => {
        if (part.type === 'start') {
          return { model: models[choice].label, routed };
        }
      },
      onError: error => (error instanceof Error ? error.message : String(error)),
    }),
  });
}
