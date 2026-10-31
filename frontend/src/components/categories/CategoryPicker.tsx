import { useTranslation } from "../../i18n/context";
import { CheckOutlined, SearchOutlined } from "../ui/icons";
import { Drawer, Input, Popover } from "../ui";
import { useMemo, useState, type ReactNode } from "react";

import { renderCategoryIcon } from "../../presentation/categoryLabels";
import type { CategoryChoice } from "../../presentation/transactionDetail";
import { categorySections, type CategorySection } from "./categorySections";
import { useCompactScreen } from "../shared/useCompactScreen";

export function CategorySearch({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <Input
      allowClear
      autoFocus
      className="category-search"
      prefix={<SearchOutlined aria-hidden="true" />}
      placeholder={t("searchCategoryNoEllipsis")}
      aria-label={t("searchCategoryNoEllipsis")}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

export function CategoryOption({
  option,
  selected,
  onSelect,
}: {
  option: CategoryChoice;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <li>
      <button
        type="button"
        role="option"
        aria-selected={selected}
        className={`category-option ${selected ? "is-selected" : ""}`}
        onClick={() => onSelect(option.value)}
      >
        <span className="category-option-icon" style={{ color: option.color }} aria-hidden="true">
          {renderCategoryIcon(option.icon)}
        </span>
        <span className="category-option-name">
          {option.name}
          {!option.isActive && <span className="category-option-note">{t("inactiveCategory")}</span>}
        </span>
        {selected && <CheckOutlined className="category-option-check" aria-hidden="true" />}
      </button>
    </li>
  );
}

export function CategoryPickerList({
  sections,
  value,
  onSelect,
}: {
  sections: CategorySection[];
  value: string | null;
  onSelect: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (sections.length === 0) {
    return <p className="category-list-empty">{t("noCategoriesFound")}</p>;
  }
  return (
    <div className="category-list" role="listbox" aria-label={t("categories")}>
      {sections.map((section) => (
        <section key={section.key} className="category-list-section">
          <h4 className="category-list-title">{section.title}</h4>
          <ul>
            {section.options.map((option) => (
              <CategoryOption
                key={`${section.key}:${option.value}`}
                option={option}
                selected={option.value === value}
                onSelect={onSelect}
              />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

/**
 * Chooses a category. The data, the ordering and the search are shared;
 * only the container follows the viewport: a popover with its own scroll
 * from `md` up, a dedicated bottom sheet on a phone.
 *
 * `children` renders the trigger. Selection applies at once and closes the
 * picker; the search resets with it. On a wide viewport the Popover wires
 * the trigger's click itself, so `toggle` is a no-op there and the trigger
 * must not stop propagation.
 */
export function CategoryPicker({
  value,
  options,
  suggestedId = null,
  recentIds = [],
  onSelect,
  disabled = false,
  children,
}: {
  value: string | null;
  options: CategoryChoice[];
  suggestedId?: string | null;
  recentIds?: string[];
  onSelect: (id: string) => void;
  disabled?: boolean;
  children: (props: { open: boolean; toggle: () => void }) => ReactNode;
}) {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const sections = useMemo(
    () => categorySections({ options, suggestedId, recentIds, search, language }),
    [options, suggestedId, recentIds, search, language],
  );

  const close = () => {
    setOpen(false);
    setSearch("");
  };
  const toggle = () => {
    if (disabled) return;
    if (open) close();
    else setOpen(true);
  };
  const select = (id: string) => {
    close();
    onSelect(id);
  };

  const body = (
    <>
      <div className="category-picker-search">
        <CategorySearch value={search} onChange={setSearch} />
      </div>
      <div className="category-picker-scroll">
        <CategoryPickerList sections={sections} value={value} onSelect={select} />
      </div>
    </>
  );

  if (compact) {
    return (
      <>
        {children({ open, toggle })}
        <Drawer
          open={open}
          onClose={close}
          placement="bottom"
          height="80vh"
          title={t("selectCategory")}
          className="category-sheet"
          destroyOnHidden
        >
          {body}
        </Drawer>
      </>
    );
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => (next ? !disabled && setOpen(true) : close())}
      trigger="click"
      placement="bottomLeft"
      arrow={false}
      destroyOnHidden
      classNames={{ root: "category-popover" }}
      content={<div className="category-popover-body">{body}</div>}
    >
      {children({ open, toggle: () => undefined })}
    </Popover>
  );
}
