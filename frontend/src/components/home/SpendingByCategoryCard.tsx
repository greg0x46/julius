import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import { Skeleton } from "../ui";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { ReportValuation } from "../shared/ReportValuation";
import { ReportingDetails } from "../shared/ReportingDetails";
import { reportingLabel } from "../../presentation/reporting";
import type {
  CategoryBreakdown,
  CategoryDirection,
  CategorySpendingItem,
} from "../../api/contracts";
import { useCategoryBreakdown } from "../../hooks/useCategoryBreakdown";
import type { Period } from "../../hooks/usePeriod";
import {
  categoryPalette,
  otherCategoriesColor,
  uncategorizedColor,
} from "../../presentation/chartColors";
import { formatReportMoney, sumBRL } from "../../presentation/money";
import { UnavailableState } from "../AsyncState";
import { filtersToSearchParams } from "../filters/filterUrl";
import { PageTabs, Section } from "../layout";


/** Categories listed individually; the rest fold into one "Outras" row. */
const maxListedCategories = categoryPalette.length;

function percent(amount: number, total: number, language: Language) {
  const value = total > 0 ? (amount / total) * 100 : 0;
  return value > 0 && value < 0.1
    ? language === "en" ? "<0.1%" : "<0,1%"
    : `${new Intl.NumberFormat(language, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(value)}%`;
}

function transactionLink(
  period: Period,
  classification: CategoryDirection,
  item?: CategorySpendingItem,
) {
  const params = filtersToSearchParams({
    ...(period.from === null
      ? { period: "all" }
      : { date_from: period.from, date_to: period.to }),
    classification,
    category_ids: item?.category_id ? [item.category_id] : [],
    uncategorized: item ? item.category_id === null : null,
  });
  return `/transacoes?${params}`;
}

interface Slice {
  key: string;
  name: string;
  amount: string;
  value: number;
  color: string;
  /** Absent for the folded "Outras" slice, which links to the whole list. */
  item?: CategorySpendingItem;
}

/**
 * The biggest categories one by one, the tail folded into "Outras": a donut
 * with thirty slivers and a list that scrolls inside the page are both noise,
 * and the folded slice keeps the percentages adding up to the whole. Colours
 * follow the order in this list (see `categoryPalette`, which keeps the
 * income/expense green and red out of it); "Sem categoria" is a neutral
 * because it is the absence of a category, not one more.
 */
function toSlices(items: CategorySpendingItem[], language: Language): Slice[] {
  const sorted = [...items].sort((a, b) => Number(b.amount) - Number(a.amount));
  const listed = sorted.slice(0, maxListedCategories);
  const rest = sorted.slice(maxListedCategories);
  let nextColor = 0;
  const slices: Slice[] = listed.map((item) => ({
    key: item.category_id ?? "uncategorized",
    name: item.category_id === null ? translate(language, "uncategorized") : item.category_name,
    amount: item.amount,
    value: Number(item.amount),
    color:
      item.category_id === null
        ? uncategorizedColor
        : categoryPalette[nextColor++ % categoryPalette.length],
    item,
  }));
  if (rest.length > 0) {
    const amount = sumBRL(rest.map((item) => item.amount));
    slices.push({
      key: "others",
      name: translate(language, "otherCategories").replace("{count}", String(rest.length)),
      amount,
      value: Number(amount),
      color: otherCategoriesColor,
    });
  }
  return slices;
}

/** The donut's centre figure shrinks with its length instead of breaking mid-number. */
function donutFigureSize(text: string): string {
  if (text.length <= 14) return "var(--fs-row)";
  if (text.length <= 18) return "var(--fs-body)";
  return "var(--fs-caption)";
}

function CategorySkeleton() {
  const { t } = useTranslation();
  return (
    <div role="status" aria-label={t("loadingCategories")}>
      <div className="category-skeleton-donut">
        <Skeleton.Avatar active shape="circle" size={160} />
      </div>
      <Skeleton active title={false} paragraph={{ rows: 4 }} />
    </div>
  );
}

function Breakdown({
  data,
  period,
  direction,
}: {
  data: CategoryBreakdown;
  period: Period;
  direction: CategoryDirection;
}) {
  const { t, language } = useTranslation();
  const [active, setActive] = useState<string | null>(null);
  const navigate = useNavigate();
  const total = Number(data.total);
  const format = (value: string) =>
    formatReportMoney(value, data.currency_code);
  const slices = toSlices(data.items, language);
  const linkFor = (slice: Slice) =>
    transactionLink(period, direction, slice.item);

  if (total <= 0 || slices.length === 0) {
    return (
      <div className="category-empty">
        <strong>{format("0.00")}</strong>
        <p>
          {t(direction === "outflow" ? "noOutflowsPeriod" : "noInflowsPeriod")}
        </p>
      </div>
    );
  }

  return (
    <div className="category-breakdown">
      <div className="category-donut">
        <div className="category-donut-total">
          <span>{t(direction === "outflow" ? "totalOutflows" : "totalInflows")}</span>
          <strong
            title={format(data.total ?? "0")}
            style={{ fontSize: donutFigureSize(format(data.total ?? "0")) }}
          >
            {format(data.total ?? "0")}
          </strong>
        </div>
        <svg
          viewBox="0 0 200 200"
          width="100%"
          height={200}
          role="img"
          aria-label={t(direction === "outflow" ? "outflowDistribution" : "inflowDistribution")}
        >
          {slices.map((slice, index) => {
            const start = slices
              .slice(0, index)
              .reduce((sum, item) => sum + item.value / total, 0);
            const fraction = slice.value / total;
            return (
              <circle
                key={slice.key}
                cx={100}
                cy={100}
                r={82}
                pathLength={1}
                fill="none"
                stroke={slice.color}
                strokeWidth={22}
                strokeDasharray={`${Math.max(0, fraction - 0.003)} ${1 - Math.max(0, fraction - 0.003)}`}
                strokeDashoffset={-start}
                transform="rotate(-90 100 100)"
                opacity={active === null || active === slice.key ? 1 : 0.35}
                style={{ cursor: "pointer" }}
                onMouseEnter={() => setActive(slice.key)}
                onMouseLeave={() => setActive(null)}
                onClick={() => navigate(linkFor(slice))}
              >
                <title>
                  {slice.name}: {format(slice.amount)} ·{" "}
                  {percent(slice.value, total, language)}
                </title>
              </circle>
            );
          })}
        </svg>
      </div>
      <ul
        className="category-breakdown-list"
        aria-label={t(direction === "outflow" ? "outflowCategories" : "inflowCategories")}
      >
        {slices.map((slice) => (
          <li key={slice.key} data-active={active === slice.key}>
            <Link
              to={linkFor(slice)}
              onFocus={() => setActive(slice.key)}
              onBlur={() => setActive(null)}
              onMouseEnter={() => setActive(slice.key)}
              onMouseLeave={() => setActive(null)}
            >
              <span
                className="category-breakdown-swatch"
                style={{ background: slice.color }}
                aria-hidden="true"
              />
              <span className="category-breakdown-name">{slice.name}</span>
              <span className="category-breakdown-amount">
                <strong>{format(slice.amount)}</strong>
                <span>{percent(slice.value, total, language)}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function SpendingByCategoryCard({ period }: { period: Period }) {
  const { t, language } = useTranslation();
  const [direction, setDirection] = useState<CategoryDirection>("outflow");
  const { data, isLoading, error, refetch } = useCategoryBreakdown(
    period,
    direction,
  );

  return (
    <Section
      title={t("byCategory")}
      trailing={
        <Link className="touch-link" to={transactionLink(period, direction)}>
          {t("viewTransactions")}
        </Link>
      }
    >
      <div className="page-tabs">
        <PageTabs
          label={t("movementType")}
          value={direction}
          onChange={setDirection}
          options={[
            { value: "outflow", label: t("outflows") },
            { value: "inflow", label: t("inflows") },
          ]}
        />
      </div>
      <div aria-live="polite" aria-busy={isLoading}>
        {isLoading ? (
          <CategorySkeleton />
        ) : error || !data ? (
          <UnavailableState onRetry={() => void refetch()}>
            {t("categoriesLoadError")}
          </UnavailableState>
        ) : data.total === null ? (
          <div role="status">
            <p>
              {data.reporting
                ? reportingLabel(data.reporting, language)
                : t("totalUnavailable")}
            </p>
            <ReportingDetails report={data.reporting} totals={data.totals} />
          </div>
        ) : (
          <ReportValuation valuation={data}>
            <ReportingDetails report={data.reporting} totals={data.totals} />
            <Breakdown
              key={`${period.from}-${period.to}-${direction}`}
              data={data}
              period={period}
              direction={direction}
            />
          </ReportValuation>
        )}
      </div>
    </Section>
  );
}
