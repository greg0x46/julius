import { DownOutlined } from "../ui/icons";
import { Button, Spin } from "../ui";
import { useState } from "react";

import { renderCategoryIcon } from "../../presentation/categoryLabels";
import type { CategoryChoice } from "../../presentation/transactionDetail";
import { CategoryPicker } from "./CategoryPicker";
import { readRecentCategories, rememberRecentCategory } from "./recentCategories";

/**
 * The category control of a transaction: the current choice, the picker
 * behind it, and — when the provider hinted at a category the person could
 * pick — a one-tap "Usar". Picks made here feed the picker's Recentes.
 */
export function CategoryField({
  id,
  value,
  options,
  suggested = null,
  onChange,
  loading = false,
  disabled = false,
}: {
  id: string;
  value: string | null;
  options: CategoryChoice[];
  /** The provider's suggestion resolved to one of `options`, if any. */
  suggested?: CategoryChoice | null;
  onChange: (id: string) => void;
  loading?: boolean;
  disabled?: boolean;
}) {
  const [recentIds, setRecentIds] = useState<string[]>(() => readRecentCategories());
  const current = options.find((option) => option.value === value) ?? null;

  const pick = (next: string) => {
    setRecentIds(rememberRecentCategory(next));
    onChange(next);
  };

  return (
    <div className="category-field">
      <CategoryPicker
        value={value}
        options={options}
        suggestedId={suggested?.value ?? null}
        recentIds={recentIds}
        onSelect={pick}
        disabled={disabled || loading}
      >
        {({ open, toggle }) => (
          <button
            type="button"
            id={id}
            className={`category-field-trigger ${current ? "" : "is-placeholder"}`}
            aria-label="Categoria"
            aria-haspopup="listbox"
            aria-expanded={open}
            disabled={disabled || loading}
            onClick={toggle}
          >
            {current ? (
              <span className="category-field-value">
                <span className="category-option-icon" style={{ color: current.color }} aria-hidden="true">
                  {renderCategoryIcon(current.icon)}
                </span>
                <span className="category-option-name">
                  {current.name}
                  {!current.isActive && <span className="category-option-note"> (inativa)</span>}
                </span>
              </span>
            ) : (
              <span className="category-field-value">Sem categoria</span>
            )}
            {loading ? <Spin size="small" /> : <DownOutlined aria-hidden="true" />}
          </button>
        )}
      </CategoryPicker>
      {suggested && suggested.value !== value && (
        <div className="category-field-suggestion">
          <span>
            <span aria-hidden="true">✨</span> Sugestão: <strong>{suggested.name}</strong>
          </span>
          <Button type="link" size="small" disabled={disabled || loading} onClick={() => pick(suggested.value)}>
            Usar
          </Button>
        </div>
      )}
    </div>
  );
}
