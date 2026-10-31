import { useTranslation } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { RightOutlined } from "../components/ui/icons";

import { ListRow, Page, Section } from "../components/layout";

interface SettingsEntry {
  path: string;
  title: MessageKey;
  description: MessageKey;
}

interface SettingsGroup {
  title: MessageKey;
  entries: SettingsEntry[];
}

// Ordered by how often each is used, grouped by what it is about — not by
// the order the screens were built in.
const groups: SettingsGroup[] = [
  {
    title: "organization",
    entries: [
      { path: "categorias", title: "categories", description: "categoriesDescription" },
      { path: "automacoes", title: "automation", description: "automationDescription" },
      { path: "cenarios", title: "scenarios", description: "scenariosDescription" },
    ],
  },
  {
    title: "data",
    entries: [
      { path: "open-banking", title: "openBanking", description: "openBankingDescription" },
      {
        path: "ativos-de-investimento",
        title: "investmentAssets",
        description: "investmentAssetsDescription",
      },
    ],
  },
  {
    title: "account",
    entries: [
      { path: "geral", title: "general", description: "generalDescription" },
      { path: "seguranca", title: "security", description: "securityDescription" },
    ],
  },
];

/**
 * The settings hub: every screen one tap away, grouped under quiet headings.
 * Rows are plain links (title, one meta line and a chevron saying "this
 * navigates"), not cards with icons.
 */
export function SettingsPage() {
  const { t } = useTranslation();
  return (
    <Page title={t("settings")} compactMobileHeader width="narrow">
      {groups.map((group) => (
        <Section key={group.title} title={t(group.title)} className="settings-group">
          <ul className="list-rows" aria-label={t(group.title)}>
            {group.entries.map((entry) => (
              <ListRow
                key={entry.path}
                title={t(entry.title)}
                meta={t(entry.description)}
                trailing={<RightOutlined className="settings-row-chevron" aria-hidden="true" />}
                href={`/configuracoes/${entry.path}`}
                ariaLabel={`${t("openItem")} ${t(entry.title)}`}
              />
            ))}
          </ul>
        </Section>
      ))}
    </Page>
  );
}
