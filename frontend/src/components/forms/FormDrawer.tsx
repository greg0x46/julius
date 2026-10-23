import { CloseOutlined } from "../ui/icons";
import { Alert, Button, Drawer } from "../ui";
import { useId, type FormEvent, type ReactNode } from "react";

import { useCompactScreen } from "../shared/useCompactScreen";

interface FormDrawerProps {
  open: boolean;
  title: ReactNode;
  onClose: () => void;
  /** Called when the form is submitted — by the Salvar button or by Enter in a field. */
  onSubmit: () => void;
  submitLabel?: string;
  /** The write is in flight: the submit button shows a spinner and ignores clicks. */
  submitting?: boolean;
  submitDisabled?: boolean;
  /** A submit or validation error, shown as an Alert above the fields. */
  error?: ReactNode;
  children: ReactNode;
  /** Wide-screen width in px. Defaults to the system's one standard width. */
  width?: number;
}

const STANDARD_WIDTH = 480;

/**
 * The shell of every create/edit form. The body is a real `<form>`, so Enter
 * submits. On wide screens it is a right-hand drawer with Cancelar + Salvar
 * in the footer; on compact screens it is full-screen, with the close button
 * at the right of the header and one full-width Salvar pinned above the
 * safe area. The footer sits outside the form's DOM, so the Salvar button
 * joins the form through the `form` attribute.
 */
export function FormDrawer({
  open,
  title,
  onClose,
  onSubmit,
  submitLabel = "Salvar",
  submitting = false,
  submitDisabled = false,
  error,
  children,
  width = STANDARD_WIDTH,
}: FormDrawerProps) {
  const compact = useCompactScreen();
  const formId = useId();

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    // A drawer opened from inside another form (e.g. "Conciliar" in the
    // recurrence editor) renders in a portal, but React still bubbles its
    // submit to the outer form — stop it here.
    event.stopPropagation();
    if (submitting || submitDisabled) return;
    onSubmit();
  };

  return (
    <Drawer
      title={title}
      open={open}
      onClose={onClose}
      width={compact ? "100%" : width}
      rootClassName="form-drawer"
      destroyOnHidden
      closable={!compact}
      extra={
        compact ? (
          <Button
            type="text"
            className="form-drawer-close"
            icon={<CloseOutlined />}
            aria-label="Fechar"
            onClick={onClose}
          />
        ) : undefined
      }
      footer={
        <div className="form-drawer-footer">
          {!compact && <Button onClick={onClose}>Cancelar</Button>}
          <Button
            type="primary"
            htmlType="submit"
            form={formId}
            size={compact ? "large" : "middle"}
            loading={submitting}
            disabled={submitDisabled}
          >
            {submitLabel}
          </Button>
        </div>
      }
    >
      <form id={formId} className="form-drawer-form" onSubmit={handleSubmit} noValidate>
        {error ? <Alert type="error" showIcon message={error} /> : null}
        {children}
      </form>
    </Drawer>
  );
}
