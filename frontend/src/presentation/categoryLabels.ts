import { translate, type Language } from "../i18n/messages";
import {
  BankOutlined,
  BulbOutlined,
  CarOutlined,
  CloudOutlined,
  CoffeeOutlined,
  CreditCardOutlined,
  CustomerServiceOutlined,
  DollarOutlined,
  EllipsisOutlined,
  FireOutlined,
  FundOutlined,
  GiftOutlined,
  GlobalOutlined,
  HeartOutlined,
  HomeOutlined,
  HourglassOutlined,
  LaptopOutlined,
  LineChartOutlined,
  MedicineBoxOutlined,
  MoneyCollectOutlined,
  PercentageOutlined,
  PhoneOutlined,
  PlusCircleOutlined,
  ReadOutlined,
  RiseOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  ScissorOutlined,
  ShoppingCartOutlined,
  ShoppingOutlined,
  SmileOutlined,
  SwapOutlined,
  TeamOutlined,
  ThunderboltOutlined,
  ToolOutlined,
  TrophyOutlined,
  WalletOutlined,
  WifiOutlined,
} from "../components/ui/icons";
import { createElement, type ComponentType } from "react";

import type { CategoryKind, CategoryOption, InternalCategory, TransactionItem } from "../api/contracts";
import type { FilterOption } from "../components/filters/ConfigurableFilters";

// Curated icon set a category's `icon` field must be one of — kept in sync
// by hand with validCategoryIcons in internal/httpapi/categories_handlers.go,
// since there's no schema shared between the Go backend and this frontend.
export const categoryIconRegistry: Record<string, ComponentType> = {
  "shopping-cart": ShoppingCartOutlined,
  coffee: CoffeeOutlined,
  car: CarOutlined,
  shopping: ShoppingOutlined,
  bank: BankOutlined,
  home: HomeOutlined,
  smile: SmileOutlined,
  percentage: PercentageOutlined,
  medicine: MedicineBoxOutlined,
  scissor: ScissorOutlined,
  team: TeamOutlined,
  tool: ToolOutlined,
  wifi: WifiOutlined,
  book: ReadOutlined,
  "safety-certificate": SafetyCertificateOutlined,
  heart: HeartOutlined,
  global: GlobalOutlined,
  gift: GiftOutlined,
  "line-chart": LineChartOutlined,
  ellipsis: EllipsisOutlined,
  "money-collect": MoneyCollectOutlined,
  laptop: LaptopOutlined,
  trophy: TrophyOutlined,
  rise: RiseOutlined,
  rollback: RollbackOutlined,
  wallet: WalletOutlined,
  "plus-circle": PlusCircleOutlined,
  swap: SwapOutlined,
  phone: PhoneOutlined,
  fund: FundOutlined,
  dollar: DollarOutlined,
  cloud: CloudOutlined,
  hourglass: HourglassOutlined,
  thunderbolt: ThunderboltOutlined,
  "credit-card": CreditCardOutlined,
  "customer-service": CustomerServiceOutlined,
  fire: FireOutlined,
  bulb: BulbOutlined,
};

// Curated hex palette a category's `color` field must be one of — kept in
// sync by hand with validCategoryColors in
// internal/httpapi/categories_handlers.go.
export const categoryColorPalette: string[] = [
  "#2a78d6",
  "#eb6834",
  "#17a2b8",
  "#e64980",
  "#d64545",
  "#b8860b",
  "#eda100",
  "#495057",
  "#37b24d",
  "#e87ba4",
  "#4c6ef5",
  "#6c757d",
  "#099268",
  "#7c5cbf",
  "#1baf7a",
  "#f76707",
];

/** Renders a category's icon by key, falling back to a generic icon for unknown/legacy keys. */
export function renderCategoryIcon(iconKey: string | undefined, props?: Record<string, unknown>) {
  const Icon = (iconKey && categoryIconRegistry[iconKey]) || EllipsisOutlined;
  return createElement(Icon, props);
}

export const categoryKindColor: Record<CategoryKind, string> = {
  expense: "red",
  income: "green",
  transfer: "blue",
};

export function internalCategoryOriginLabel(category: InternalCategory, language: Language = "pt-BR"): string {
  switch (category.origin) {
    case "manual":
      return translate(language, "manualCategorization");
    case "learned":
      return translate(language, "learnedCategorization");
    default:
      return translate(language, "automaticCategorization");
  }
}

export function internalCategoryName(item: Pick<TransactionItem, "internal_category">, language: Language = "pt-BR"): string {
  return item.internal_category?.name ?? translate(language, "uncategorized");
}

export function categoryKindName(kind: CategoryKind, language: Language = "pt-BR"): string {
  return translate(language, kind === "expense" ? "expenseCategory" : kind === "income" ? "incomeCategory" : "transferCategory");
}

export function categoryFilterOptions(categories: CategoryOption[] | undefined, language: Language = "pt-BR"): FilterOption[] {
  if (!categories) return [];
  const byKind = ["expense", "income", "transfer"] as const;
  const sorted = [...categories].sort((a, b) => {
    if (a.kind !== b.kind) return byKind.indexOf(a.kind) - byKind.indexOf(b.kind);
    if (a.is_active !== b.is_active) return a.is_active ? -1 : 1;
    return a.name.localeCompare(b.name, "pt-BR");
  });
  return sorted.map((option) => ({
    value: option.id,
    label: `${categoryKindName(option.kind, language)}: ${option.name}${option.is_active ? "" : translate(language, "inactiveCategory")}`,
    icon: option.icon,
    color: option.color,
  }));
}
