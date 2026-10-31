import { storedLanguage } from "./context";
import { translate, type MessageKey } from "./messages";

/** For error text built outside React: reads the language the user selected at throw time. */
export function apiMessage(key: MessageKey, values?: Record<string, string | number>): string {
  return translate(storedLanguage(), key, values);
}
