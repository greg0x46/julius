import { Alert, Button } from "../ui";

/** A write's failure (or its "saved, refresh pending" aftermath) with the one action that recovers from it. */
export function RetryAlert({
  type,
  message,
  detail,
  action,
  onAction,
}: {
  type: "error" | "warning";
  message: string;
  detail: string;
  action: string;
  onAction: () => void;
}) {
  return (
    <Alert
      type={type}
      showIcon
      message={message}
      description={
        <>
          <p>{detail}</p>
          <Button onClick={onAction}>{action}</Button>
        </>
      }
    />
  );
}
