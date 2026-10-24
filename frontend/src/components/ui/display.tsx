import {
  Children,
  createElement,
  useState,
  type CSSProperties,
  type HTMLAttributes,
  type MouseEvent,
  type ReactNode,
} from "react";

type Box = HTMLAttributes<HTMLDivElement> & { children?: ReactNode };
export function Flex({
  vertical,
  gap,
  align,
  justify,
  wrap,
  ...props
}: Box & {
  vertical?: boolean;
  gap?: string | number;
  align?: string;
  justify?: string;
  wrap?: boolean | "wrap" | "nowrap";
}) {
  const spacing: Record<string, number> = { small: 8, middle: 16, large: 24 };
  return (
    <div
      {...props}
      style={{
        display: "flex",
        flexDirection: vertical ? "column" : "row",
        gap: typeof gap === "string" ? (spacing[gap] ?? gap) : gap,
        alignItems: align,
        justifyContent: justify,
        flexWrap: wrap === true ? "wrap" : wrap || undefined,
        ...props.style,
      }}
    />
  );
}
export function Space({
  direction,
  size,
  ...props
}: Box & {
  direction?: string;
  size?: string | number;
  wrap?: boolean;
  align?: string;
}) {
  return (
    <Flex
      {...props}
      vertical={direction === "vertical"}
      gap={size ?? "small"}
    />
  );
}
export function Card({
  children,
  styles,
  ...props
}: Box & { styles?: { body?: CSSProperties } }) {
  return (
    <div {...props} className={`ui-card ${props.className ?? ""}`}>
      <div className="ui-card-body" style={styles?.body}>
        {children}
      </div>
    </div>
  );
}
export function Alert({
  message,
  description,
  action,
  type = "info",
  showIcon,
  closable,
  onClose,
  ...props
}: Omit<Box, "title"> & {
  message?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  type?: string;
  showIcon?: boolean;
  closable?: boolean;
  onClose?: () => void;
}) {
  const [hidden, setHidden] = useState(false);
  return hidden ? null : (
    <div
      {...props}
      role={type === "error" ? "alert" : "status"}
      className={`ui-alert ui-alert-${type} ${props.className ?? ""}`}
    >
      {showIcon && (
        <span aria-hidden="true">{type === "error" ? "!" : "ⓘ"}</span>
      )}
      <div>
        <div className="ui-alert-message">{message}</div>
        {description && <div>{description}</div>}
      </div>
      {action}
      {closable && (
        <button
          type="button"
          aria-label="Fechar aviso"
          onClick={() => {
            setHidden(true);
            onClose?.();
          }}
        >
          ×
        </button>
      )}
    </div>
  );
}
export function Tag({
  children,
  color,
  closable,
  onClose,
  closeIcon,
  ...props
}: HTMLAttributes<HTMLSpanElement> & {
  color?: string;
  closable?: boolean;
  onClose?: (event: MouseEvent<HTMLButtonElement>) => void;
  closeIcon?: ReactNode;
}) {
  return (
    <span
      {...props}
      className={`ui-tag ${props.className ?? ""}`}
      data-color={color}
    >
      {children}
      {closable && (
        <button type="button" aria-label="Remover filtro" onClick={onClose}>
          {closeIcon ?? "×"}
        </button>
      )}
    </span>
  );
}
export function Divider({
  children,
  style,
}: {
  children?: ReactNode;
  orientation?: string;
  plain?: boolean;
  style?: CSSProperties;
}) {
  return children ? (
    <div className="ui-divider" style={style}>
      {children}
    </div>
  ) : (
    <hr className="ui-divider" style={style} />
  );
}
export function Spin({
  children,
  tip,
  size,
}: {
  children?: ReactNode;
  tip?: ReactNode;
  size?: string;
}) {
  void size;
  return (
    <span aria-hidden="true" className="ui-spin">
      <span className="ui-spinner" aria-hidden="true" />
      {tip ?? <span className="visually-hidden">Carregando…</span>}
      {children}
    </span>
  );
}
export function SkeletonRoot({
  children,
  title = true,
  paragraph = { rows: 3 },
  ...props
}: Omit<Box, "title"> & {
  active?: boolean;
  title?: boolean | { width?: string };
  paragraph?: boolean | { rows?: number };
}) {
  const { active, ...rest } = props;
  void active;
  return (
    <div
      {...rest}
      className={`ui-skeleton ${props.className ?? ""}`}
      aria-hidden="true"
    >
      {children ?? (
        <>
          {title && (
            <div
              className="ui-skeleton-line"
              style={{ width: typeof title === "object" ? title.width : "40%" }}
            />
          )}
          {paragraph &&
            Array.from(
              {
                length:
                  typeof paragraph === "object" ? (paragraph.rows ?? 3) : 3,
              },
              (_, i) => <div key={i} className="ui-skeleton-line" />,
            )}
        </>
      )}
    </div>
  );
}
export function SkeletonBlock({
  active,
  block,
  shape,
  size,
  ...props
}: Box & { active?: boolean; block?: boolean; shape?: string; size?: number }) {
  void active;
  return (
    <div
      {...props}
      className="ui-skeleton-block"
      style={{
        width: block ? "100%" : size,
        height: size,
        borderRadius: shape === "circle" ? "var(--radius-full)" : "var(--radius-sm)",
        ...props.style,
      }}
      aria-hidden="true"
    />
  );
}
type TextProps = Omit<HTMLAttributes<HTMLElement>, "title"> & {
  type?: string;
  strong?: boolean;
  code?: boolean;
  copyable?: boolean;
  level?: number;
  title?: string;
  ellipsis?: boolean;
  delete?: boolean;
};
function text(
  tag: string,
  {
    type,
    strong,
    code,
    copyable,
    level,
    ellipsis,
    delete: deleted,
    ...props
  }: TextProps,
) {
  void copyable;
  void level;
  void ellipsis;
  return createElement(code ? "code" : deleted ? "del" : tag, {
    ...props,
    className: `ui-typography ${type ? `ui-typography-${type}` : ""} ${props.className ?? ""}`,
    style: { fontWeight: strong ? 600 : undefined, ...props.style },
  });
}
export function Text(props: TextProps) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  if (!props.copyable) return text("span", props);
  return (
    <span className="ui-copyable">
      {text("span", props)}
      <button
        type="button"
        aria-label={copied ? "Copiado" : "Copiar identificador"}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(String(props.children ?? ""));
            setCopied(true);
            setFailed(false);
          } catch {
            setFailed(true);
          }
        }}
      >
        {copied ? "Copiado" : "Copiar"}
      </button>
      {failed && (
        <span role="status">
          Não foi possível copiar. Selecione o texto para copiar.
        </span>
      )}
    </span>
  );
}
export function Paragraph(props: TextProps) {
  return text("p", props);
}
export function Title(props: TextProps) {
  return text(`h${props.level ?? 2}`, props);
}
export function TextLink(props: TextProps & { href?: string }) {
  return text("a", props);
}
export function Collapse({
  items,
  defaultActiveKey,
  ...props
}: {
  items: { key: string; label: ReactNode; children: ReactNode }[];
  defaultActiveKey?: string[];
  ghost?: boolean;
  size?: string;
  className?: string;
}) {
  return (
    <div className={`ui-collapse ${props.className ?? ""}`}>
      {items.map((item) => (
        <details key={item.key} open={defaultActiveKey?.includes(item.key)}>
          <summary>{item.label}</summary>
          {item.children}
        </details>
      ))}
    </div>
  );
}
export function DescriptionItem({
  label,
  children,
}: {
  label: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="ui-descriptions-item">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}
export function DescriptionsRoot({
  children,
}: {
  children: ReactNode;
  column?: number;
  size?: string;
  colon?: boolean;
}) {
  return <dl className="ui-descriptions">{children}</dl>;
}
export function ListRoot<T>({
  dataSource,
  renderItem,
}: {
  size?: string;
  dataSource: T[];
  renderItem: (item: T, index: number) => ReactNode;
}) {
  return (
    <ul className="ui-list">
      {dataSource.map((item, index) => (
        <li key={index}>{renderItem(item, index)}</li>
      ))}
    </ul>
  );
}
export function ListItem({
  children,
  actions,
}: {
  children: ReactNode;
  actions?: ReactNode[];
}) {
  return (
    <div className="ui-list-item">
      {children}
      <span>{Children.toArray(actions)}</span>
    </div>
  );
}
