import { useTranslation } from "../../i18n/context";
import { FilterOutlined } from "../ui/icons";
import { Button } from "../ui";

interface FilterButtonProps {
  /** How many filters are active; shown next to the label and in the name. */
  activeCount?: number;
  onClick: () => void;
}

/**
 * Opens the advanced filters. Its accessible name carries the active count
 * ("Filtros 2") so the state is announced, and on a phone the text collapses
 * to the icon while the count stays.
 */
export function FilterButton({ activeCount = 0, onClick }: FilterButtonProps) {
  const { t } = useTranslation();
  const name = `${t("filters")}${activeCount ? ` ${activeCount}` : ""}`;
  return (
    <Button
      className="list-toolbar-filter-button"
      aria-label={name}
      title={activeCount ? t("activeFiltersTitle", { count: activeCount }) : t("filters")}
      icon={<FilterOutlined aria-hidden="true" />}
      onClick={onClick}
    >
      <span className="list-toolbar-filter-label">{t("filters")}</span>
      {activeCount ? <span className="list-toolbar-filter-count">{activeCount}</span> : null}
    </Button>
  );
}
