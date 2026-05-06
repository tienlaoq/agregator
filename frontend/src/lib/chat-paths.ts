/** Префикс HTTP API чата v2 под монтированием `/api/v1`. */
export const CHAT_V2_PREFIX = "/api/v1/v2/chat" as const;

/** Путь WebSocket чата (относительно origin gateway). */
export const CHAT_V2_WS_PATH = `${CHAT_V2_PREFIX}/ws` as const;

export const chatV2Paths = {
  threads: `${CHAT_V2_PREFIX}/threads`,
  threadsEnsure: `${CHAT_V2_PREFIX}/threads:ensure`,
  threadMessages: (threadId: string) =>
    `${CHAT_V2_PREFIX}/threads/${encodeURIComponent(threadId)}/messages`,
  threadRead: (threadId: string) =>
    `${CHAT_V2_PREFIX}/threads/${encodeURIComponent(threadId)}:read`,
  wsTicket: `${CHAT_V2_PREFIX}/ws-ticket`,
} as const;
