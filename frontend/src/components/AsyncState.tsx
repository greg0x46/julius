import { useTranslation } from "../i18n/context";
import { Alert, Button, Flex, Spin } from "./ui";
import type { ReactNode } from "react";

export function LoadingState({ children }: { children?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <Flex gap="small" align="center" role="status">
      <Spin size="small" />
      <span>{children ?? t("loading")}</span>
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
  const { t } = useTranslation();
  return (
    <Alert
      type="error"
      showIcon
      message={children}
      action={<Button onClick={onRetry}>{t("retry")}</Button>}
    />
  );
}
