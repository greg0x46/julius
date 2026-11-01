import { translate, type Language, type MessageKey } from "../../i18n/messages";
import { fold, type CategoryChoice } from "../../presentation/transactionDetail";

const kindOrder: CategoryChoice["kind"][] = ["expense", "income", "transfer"];
const kindGroupTitle: Record<CategoryChoice["kind"], MessageKey> = {
  expense: "expenseCategories",
  income: "incomeCategories",
  transfer: "transferCategories",
};

export type CategorySection = { key: string; title: string; options: CategoryChoice[] };

/**
 * The one set of rules behind both presentations: what the list shows, in
 * which order, and what the search matches. Sections are, in order:
 * Sugeridas (the provider's hint), Recentes (last manual picks on this
 * device), Todas — grouped by nature only when more than one applies.
 */
export function categorySections({
  options,
  suggestedId,
  recentIds,
  search,
  language = "pt-BR",
}: {
  options: CategoryChoice[];
  suggestedId: string | null;
  recentIds: string[];
  search: string;
  language?: Language;
}): CategorySection[] {
  const query = fold(search);
  const matches = query ? options.filter((option) => fold(option.name).includes(query)) : options;
  const byId = new Map(matches.map((option) => [option.value, option]));

  const suggested = suggestedId ? byId.get(suggestedId) : undefined;
  const recent = recentIds
    .map((id) => byId.get(id))
    .filter((option): option is CategoryChoice => option !== undefined && option.value !== suggestedId);

  const sections: CategorySection[] = [];
  if (suggested) sections.push({ key: "suggested", title: translate(language, "suggestedCategories"), options: [suggested] });
  if (recent.length > 0) sections.push({ key: "recent", title: translate(language, "recentCategories"), options: recent });

  const kinds = new Set(matches.map((option) => option.kind));
  if (kinds.size > 1) {
    for (const kind of kindOrder) {
      const group = matches.filter((option) => option.kind === kind);
      if (group.length > 0) sections.push({ key: `all-${kind}`, title: translate(language, kindGroupTitle[kind]), options: group });
    }
  } else if (matches.length > 0) {
    sections.push({ key: "all", title: translate(language, "allFeminine"), options: matches });
  }
  return sections;
}
