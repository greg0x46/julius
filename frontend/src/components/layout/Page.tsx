import { LeftOutlined } from "../ui/icons";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { useCompactScreen } from "../shared/useCompactScreen";

interface PageProps {
  title: string;
  /**
   * Where this page was reached from, e.g. a detail page's list: renders a
   * chevron link before the title, at every width. It navigates away rather
   * than acting on this page, so it is not a page action.
   */
  backTo?: string;
  /** The chevron link's accessible name ("Voltar para contas e cartões"). */
  backLabel?: string;
  /**
   * One line under the title on wide screens. On a phone it is dropped when
   * `compactMobileHeader` is set, so pages must not rely on it for meaning.
   */
  description?: string;
  /**
   * Page context: a control that changes the whole page at once (the period
   * navigator moves cards, totals, charts and lists together). It is not a
   * filter of any one list, so it lives in the header, not in a toolbar.
   */
  context?: ReactNode;
  /**
   * Page actions: creation and other page-wide commands. They sit on the
   * title row's right edge at every width — build the primary one with
   * `PageAction` (or `CreateActionMenu`) so it turns compact on a phone.
   */
  actions?: ReactNode;
  /** Top-level views of the page, rendered right under the header. */
  tabs?: ReactNode;
  /**
   * How wide the page may grow on a big screen. `full` (default) caps at
   * 1440px, for dashboards and wide tables; `narrow` caps at 1120px, for
   * lists and settings, where a line of 1700px is unreadable. Both centre
   * beyond the cap; the header follows the same width as the content.
   */
  width?: "full" | "narrow";
  /** On a phone, drop the description for a single title line. */
  compactMobileHeader?: boolean;
  /**
   * Set when a BottomActionBar is mounted on a phone — a task flow's
   * confirmation, not a list page's create action. The page then reserves
   * room at the bottom of its scroll area for the bar (shared rule in
   * layout.css) and does not repeat `actions` in the title row.
   */
  hasBottomActionBar?: boolean;
  className?: string;
  children: ReactNode;
}

/**
 * The shell every screen composes: one title row — optional back chevron,
 * the h1, then page context and page actions at the right edge — with an
 * optional description line under it, optional tabs, then the content. The
 * title is the same size and colour at every width, and its left edge lines
 * up with the content gutter. Collection controls (search, filters,
 * grouping, sorting) never go here — they belong to a ListToolbar sitting
 * right above the list they change.
 *
 * On a phone the actions stay in the title row (compact), and the context
 * drops to its own full-width row under it.
 */
export function Page({
  title,
  backTo,
  backLabel = "Voltar",
  description,
  context,
  actions,
  tabs,
  compactMobileHeader,
  hasBottomActionBar,
  width = "full",
  className,
  children,
}: PageProps) {
  const compact = useCompactScreen();
  const bottomBar = compact && hasBottomActionBar === true;
  const hideDescription = compactMobileHeader === true && compact;
  const hasContext = context !== undefined;
  const hasVisibleActions = actions !== undefined && !bottomBar;

  return (
    <PageFrame
      // The page's name is the one heading assistive tech should land on, so
      // the header is rendered by hand: a real h1 inside a real <header>.
      pageHeaderRender={() => (
        <header className="page-header">
          <div className="page-heading">
            {backTo !== undefined && (
              <Link
                to={backTo}
                className="page-back-link"
                aria-label={backLabel}
              >
                <LeftOutlined aria-hidden="true" />
              </Link>
            )}
            {/* On a phone a long name is clamped to two lines; `title` keeps the whole of it. */}
            <h1 className="page-title" title={title}>
              {title}
            </h1>
          </div>
          {(hasContext || hasVisibleActions) && (
            <div className="page-header-slots">
              {hasContext && <div className="page-context">{context}</div>}
              {hasVisibleActions && (
                <div className="page-actions">{actions}</div>
              )}
            </div>
          )}
          {description !== undefined && !hideDescription && (
            <p className="page-description">{description}</p>
          )}
        </header>
      )}
      className={[
        "app-page",
        width === "narrow" ? "app-page-narrow" : "",
        bottomBar ? "app-page-has-bottom-bar" : "",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {tabs !== undefined && <div className="page-tabs">{tabs}</div>}
      {children}
    </PageFrame>
  );
}

function PageFrame({
  pageHeaderRender,
  className,
  children,
}: {
  pageHeaderRender: () => ReactNode;
  className: string;
  children: ReactNode;
}) {
  return (
    <div className={className}>
      {pageHeaderRender()}
      <div className="ui-page-content">{children}</div>
    </div>
  );
}
