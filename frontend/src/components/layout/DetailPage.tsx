import { useTranslation } from "../../i18n/context";
import { Alert, Button, Flex, Skeleton } from "../ui";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { UnavailableState } from "../AsyncState";
import { Page } from "./Page";

export type DetailFreshness = "loading" | "fresh" | "stale" | "not_found" | "unavailable";

/**
 * The shape every detail-page data hook already returns (useAccountDetail,
 * useInvestmentDetail, usePayableDetail, useSyncRun): a snapshot, and the
 * freshness reading that says whether it is safe to trust.
 */
export interface DetailPageQueryState<T> {
  snapshot: T | null;
  freshness: DetailFreshness;
  retrying: boolean;
}

interface DetailPageFrameProps {
  /** The generic page name: the h1 until a record is loaded, and on every
   *  state that never has one (invalid, not found, unavailable). */
  title: string;
  /** The list route this record was opened from. Page renders it as the
   *  chevron before the title, and the error states below reuse it as their
   *  recovery link (not found, invalid). */
  backTo: string;
  /** The back link's name ("Voltar para contas e cartões"). */
  backLabel: string;
  /** Passed to `Page`: `narrow` caps a big screen at 1120px. */
  width?: "full" | "narrow";
  className?: string;
}


/**
 * Shown instead of ever calling the page's data hook when the route's id
 * does not look like a real identifier — each page still runs that isUuid
 * check itself (calling a detail hook with a bad id would fire a real
 * request), this only gives the failure the same Page chrome as everything
 * else. The page's h1 is `title`; the alert states the problem without a
 * second heading.
 */
export function InvalidDetailPage({
  title,
  backTo,
  backLabel,
  width,
  className,
  invalidTitle,
  invalidDescription,
}: DetailPageFrameProps & { invalidTitle: string; invalidDescription?: string }) {
  const { t } = useTranslation();
  return (
    <Page title={title} backTo={backTo} backLabel={backLabel} width={width} className={className} compactMobileHeader>
      <Alert
        type="error"
        showIcon
        message={invalidTitle}
        description={invalidDescription ?? t("invalidIdentifier")}
        action={<Link to={backTo}>{backLabel}</Link>}
      />
    </Page>
  );
}

interface DetailPageProps<T> extends DetailPageFrameProps {
  state: DetailPageQueryState<T>;
  retry: () => void;
  /** The record's own name once loaded: it becomes the page's h1 in place of
   *  `title`, so the record is named once, in the header, and the body
   *  never repeats it as a heading. */
  recordTitle?: (snapshot: T) => string;
  loadingLabel?: ReactNode;
  notFoundMessage: string;
  notFoundDescription: string;
  unavailableMessage: ReactNode;
  /** Page actions for the title row (Editar/Excluir behind a `···`, a
   *  PageAction). Shown only once a record is loaded: before that there is
   *  nothing to act on, and not found/unavailable have their own recovery. */
  actions?: ReactNode;
  /** The loaded body. Only ever called with a non-null snapshot — render
   *  anything the page needs here, including modals/panels tied to it;
   *  before a snapshot exists there is nothing of the page's own state for
   *  them to show anyway. */
  children: (snapshot: T) => ReactNode;
}

/**
 * The detail-page shell shared by Conta, Investimento, Pendência and
 * Sincronização: title/back/actions through Page (the record's name is the
 * h1, the chevron goes back to its list), then the one
 * loading → not_found → unavailable → stale → loaded switch all four used to
 * repeat by hand. Each page still owns its own data hook and its own body
 * (via `children`) — this only owns the chrome around whatever freshness
 * that hook reports.
 */
export function DetailPage<T>({
  title,
  backTo,
  backLabel,
  width,
  className,
  state,
  retry,
  recordTitle,
  loadingLabel,
  notFoundMessage,
  notFoundDescription,
  unavailableMessage,
  actions,
  children,
}: DetailPageProps<T>) {
  const { t } = useTranslation();
  const back = <Link to={backTo}>{backLabel}</Link>;
  const pageTitle = state.snapshot !== null && recordTitle !== undefined ? recordTitle(state.snapshot) : title;
  return (
    <Page
      title={pageTitle}
      backTo={backTo}
      backLabel={backLabel}
      width={width}
      className={className}
      compactMobileHeader
      actions={state.snapshot !== null ? actions : undefined}
    >
      <Flex vertical gap="large">
        {state.freshness === "loading" && (
          // Shaped like a detail page (summary, then a section) so content
          // replaces it without the page jumping; the label stays for
          // assistive tech.
          <div role="status" className="detail-page-skeleton">
            <Skeleton active title={{ width: "35%" }} paragraph={{ rows: 2 }} />
            <Skeleton active title={{ width: "25%" }} paragraph={{ rows: 4 }} />
            <span className="visually-hidden">{loadingLabel ?? t("loading")}</span>
          </div>
        )}
        {state.freshness === "not_found" && (
          <Alert
            type="error"
            showIcon
            message={notFoundMessage}
            description={notFoundDescription}
            action={back}
          />
        )}
        {state.freshness === "unavailable" && (
          <UnavailableState onRetry={retry}>{unavailableMessage}</UnavailableState>
        )}
        {state.snapshot !== null && (
          <>
            {state.freshness === "stale" && (
              <Alert
                type="warning"
                showIcon
                message={t("informationMayBeOutdated")}
                action={
                  <Button loading={state.retrying} onClick={retry}>
                    {t("retry")}
                  </Button>
                }
              />
            )}
            {children(state.snapshot)}
          </>
        )}
      </Flex>
    </Page>
  );
}
