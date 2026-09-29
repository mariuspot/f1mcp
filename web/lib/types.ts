import type { InferUITools, UIDataTypes, UIMessage } from 'ai';
import type { F1Tools } from './tools';

// Metadata on each answer: the model that wrote it, and whether auto picked
// it.
export type ChatMetadata = { model?: string; routed?: boolean };

export type ChatMessage = UIMessage<ChatMetadata, UIDataTypes, InferUITools<F1Tools>>;
