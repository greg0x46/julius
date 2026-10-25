import { App } from "../ui";
import { useMemo } from "react";

export interface Feedback {
  /** Short confirmation of a write ("Salvo", "Transação criada"). */
  success: (text: string) => void;
  /** Transient failure. Persistent errors still belong inline (Alert) where the user is. */
  error: (text: string) => void;
  info: (text: string) => void;
}

const SUCCESS_SECONDS = 3;
const ERROR_SECONDS = 5;

const silent: Feedback = { success: () => undefined, error: () => undefined, info: () => undefined };

/** Transient feedback is provided by the application shell; isolated consumers can omit it. */
export function useFeedback(): Feedback {
  const { message } = App.useApp();
  return useMemo(() => {
    if (typeof message?.success !== "function") return silent;
    return {
      success: (text) => void message.success({ content: text, duration: SUCCESS_SECONDS }),
      error: (text) => void message.error({ content: text, duration: ERROR_SECONDS }),
      info: (text) => void message.info({ content: text, duration: SUCCESS_SECONDS }),
    };
  }, [message]);
}
