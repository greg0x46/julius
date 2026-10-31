import { useTranslation } from "../i18n/context";
import type { ReactNode } from "react";

import { Page } from "./layout";

interface SettingsPageContainerProps {
  title: string;
  /** One line under the title — mirrors Page's own `description`, the only explanatory text of the header. */
  subTitle?: string;
  /** Page-wide actions (e.g. "Nova categoria"). Rendered in Page's actions slot. */
  extra?: ReactNode;
  /** Forwarded to Page — see its own doc comment. */
  compactMobileHeader?: boolean;
  children: ReactNode;
}

/**
 * The shell every settings screen composes on top of `Page`: the section's
 * title/description and a back chevron to the settings hub. (A sync run's
 * detail is not one of these screens — it goes through `DetailPage`, with
 * its own way back to Open Banking.)
 *
 * There is no second explanatory paragraph: it used to repeat the
 * description in other words. Anything a user needs to know to act lives
 * next to the control it explains (a field hint, an empty state's hint).
 * `extra` is a page action like any other: it stays in the title row on a
 * phone, not in a bottom bar. Settings are narrow pages: a list or a form
 * across 1400px is unreadable.
 */
export function SettingsPageContainer({
  title,
  subTitle,
  extra,
  compactMobileHeader,
  children,
}: SettingsPageContainerProps) {
  const { t } = useTranslation();
  return (
    <Page
      title={title}
      description={subTitle}
      backTo="/configuracoes"
      backLabel={t("backToSettings")}
      compactMobileHeader={compactMobileHeader}
      width="narrow"
      actions={extra}
    >
      {children}
    </Page>
  );
}
