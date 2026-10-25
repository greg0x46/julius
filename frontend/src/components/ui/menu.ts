import type { ReactNode } from "react";
export interface ItemType {
  key: string;
  label: ReactNode;
  danger?: boolean;
  disabled?: boolean;
  onClick?: () => void;
  children?: ItemType[];
}
