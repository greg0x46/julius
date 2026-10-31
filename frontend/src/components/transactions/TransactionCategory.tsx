import { useTranslation } from "../../i18n/context";
import type { CSSProperties } from "react";

import type { TransactionItem } from "../../api/contracts";
import {
  internalCategoryName,
  internalCategoryOriginLabel,
  renderCategoryIcon,
} from "../../presentation/categoryLabels";

/**
 * The category as quiet text with its icon in the category colour — an
 * accent to recognise, not a pill that outshines the description.
 */
export function TransactionCategory({ item }: { item: TransactionItem }) {
  const { language } = useTranslation();
  const category = item.internal_category;
  return (
    <span
      className={`transaction-category ${category ? "" : "is-empty"}`}
      style={category ? ({ "--category-color": category.color } as CSSProperties) : undefined}
      title={category ? internalCategoryOriginLabel(category, language) : undefined}
    >
      {category && (
        <span className="transaction-category-icon" aria-hidden="true">
          {renderCategoryIcon(category.icon)}
        </span>
      )}
      <span className="transaction-category-name">{internalCategoryName(item, language)}</span>
    </span>
  );
}
