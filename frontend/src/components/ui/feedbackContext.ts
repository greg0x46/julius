import { createContext, type ReactNode } from "react";
export interface Confirm {
  title: ReactNode;
  content?: ReactNode;
  okText?: string;
  cancelText?: string;
  okButtonProps?: { danger?: boolean };
  onOk: () => unknown | Promise<unknown>;
}
export interface Messages {
  success: (options: { content: string; duration: number }) => void;
  error: (options: { content: string; duration: number }) => void;
  info: (options: { content: string; duration: number }) => void;
}
export const FeedbackContext = createContext<{
  modal?: { confirm: (options: Confirm) => void };
  message?: Messages;
}>({});
