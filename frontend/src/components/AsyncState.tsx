import { Alert, Button, Flex, Spin } from "./ui";
import type { ReactNode } from "react";

export function LoadingState({ children = "Carregando…" }: { children?: ReactNode }) {
  return (
    <Flex gap="small" align="center" role="status">
      <Spin size="small" />
      <span>{children}</span>
    </Flex>
  );
}

export function UnavailableState({
  children,
  onRetry,
}: {
  children: ReactNode;
  onRetry: () => void;
}) {
  return (
    <Alert
      type="error"
      showIcon
      message={children}
      action={<Button onClick={onRetry}>Tentar novamente</Button>}
    />
  );
}
