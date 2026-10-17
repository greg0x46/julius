import { RightOutlined } from "@ant-design/icons";

import { ListRow, Page, Section } from "../components/layout";

interface SettingsEntry {
  path: string;
  title: string;
  description: string;
}

interface SettingsGroup {
  title: string;
  entries: SettingsEntry[];
}

// Ordered by how often each is used, grouped by what it is about — not by
// the order the screens were built in.
const groups: SettingsGroup[] = [
  {
    title: "Organização",
    entries: [
      { path: "categorias", title: "Categorias", description: "Organize as categorias das suas receitas e despesas." },
      { path: "automacoes", title: "Automações", description: "Regras para organizar suas transações." },
      { path: "cenarios", title: "Cenários", description: "Simule decisões hipotéticas no seu planejamento." },
    ],
  },
  {
    title: "Dados",
    entries: [
      { path: "open-banking", title: "Open Banking", description: "Conexões, sincronizações e credenciais." },
      {
        path: "ativos-de-investimento",
        title: "Ativos de investimento",
        description: "Cadastre e edite os ativos dos seus investimentos.",
      },
    ],
  },
  {
    title: "Conta",
    entries: [
      { path: "geral", title: "Geral", description: "Moeda dos relatórios e mês das compras no cartão." },
      { path: "seguranca", title: "Segurança", description: "Senha e acesso à sua conta." },
    ],
  },
];

/**
 * The settings hub: every screen one tap away, grouped under quiet headings.
 * Rows are plain links (title, one meta line and a chevron saying "this
 * navigates"), not cards with icons.
 */
export function SettingsPage() {
  return (
    <Page title="Configurações" compactMobileHeader width="narrow">
      {groups.map((group) => (
        <Section key={group.title} title={group.title} className="settings-group">
          <ul className="list-rows" aria-label={group.title}>
            {group.entries.map((entry) => (
              <ListRow
                key={entry.path}
                title={entry.title}
                meta={entry.description}
                trailing={<RightOutlined className="settings-row-chevron" aria-hidden="true" />}
                href={`/configuracoes/${entry.path}`}
                ariaLabel={`Abrir ${entry.title}`}
              />
            ))}
          </ul>
        </Section>
      ))}
    </Page>
  );
}
