import { Skeleton } from "antd";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

import { ReportValuation } from "../shared/ReportValuation";
import { ReportingDetails } from "../shared/ReportingDetails";
import { reportingLabel } from "../../presentation/reporting";
import type { CategoryBreakdown, CategoryDirection, CategorySpendingItem } from "../../api/contracts";
import { useCategoryBreakdown } from "../../hooks/useCategoryBreakdown";
import type { Period } from "../../hooks/usePeriod";
import { categoryPalette, otherCategoriesColor, uncategorizedColor } from "../../presentation/chartColors";
import { formatReportMoney, sumBRL } from "../../presentation/money";
import { UnavailableState } from "../AsyncState";
import { filtersToSearchParams } from "../filters/filterUrl";
import { PageTabs, Section } from "../layout";

const percentFormatter = new Intl.NumberFormat("pt-BR", { minimumFractionDigits: 1, maximumFractionDigits: 1 });

/** Categories listed individually; the rest fold into one "Outras" row. */
const maxListedCategories = categoryPalette.length;

function percent(amount: number, total: number) {
  const value = total > 0 ? (amount / total) * 100 : 0;
  return value > 0 && value < 0.1 ? "<0,1%" : `${percentFormatter.format(value)}%`;
}

function transactionLink(period: Period, classification: CategoryDirection, item?: CategorySpendingItem) {
  const params = filtersToSearchParams({
    ...(period.from === null ? { period: "all" } : { date_from: period.from, date_to: period.to }),
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
function toSlices(items: CategorySpendingItem[]): Slice[] {
  const sorted = [...items].sort((a, b) => Number(b.amount) - Number(a.amount));
  const listed = sorted.slice(0, maxListedCategories);
  const rest = sorted.slice(maxListedCategories);
  let nextColor = 0;
  const slices: Slice[] = listed.map((item) => ({
    key: item.category_id ?? "uncategorized",
    name: item.category_name,
    amount: item.amount,
    value: Number(item.amount),
    color: item.category_id === null ? uncategorizedColor : categoryPalette[nextColor++ % categoryPalette.length],
    item,
  }));
  if (rest.length > 0) {
    const amount = sumBRL(rest.map((item) => item.amount));
    slices.push({
      key: "others",
      name: `Outras (${rest.length} categorias)`,
      amount,
      value: Number(amount),
      color: otherCategoriesColor,
    });
  }
  return slices;
}

/** The donut's centre figure shrinks with its length instead of breaking mid-number. */
function donutFigureSize(text: string): string {
  if (text.length <= 12) return "1.125rem";
  if (text.length <= 14) return "1rem";
  if (text.length <= 16) return "0.875rem";
  if (text.length <= 18) return "0.8125rem";
  return "0.75rem";
}

function CategorySkeleton() {
  return (
    <div role="status" aria-label="Carregando categorias">
      <div className="category-skeleton-donut">
        <Skeleton.Avatar active shape="circle" size={160} />
      </div>
      <Skeleton active title={false} paragraph={{ rows: 4 }} />
    </div>
  );
}

function Breakdown({ data, period, direction }: { data: CategoryBreakdown; period: Period; direction: CategoryDirection }) {
  const [active, setActive] = useState<string | null>(null);
  const navigate = useNavigate();
  const total = Number(data.total);
  const label = direction === "outflow" ? "saídas" : "entradas";
  const format = (value: string) => formatReportMoney(value, data.currency_code);
  const slices = toSlices(data.items);
  const linkFor = (slice: Slice) => transactionLink(period, direction, slice.item);

  if (total <= 0 || slices.length === 0) {
    return (
      <div className="category-empty">
        <strong>{format("0.00")}</strong>
        <p>Nenhuma {direction === "outflow" ? "saída" : "entrada"} neste período.</p>
      </div>
    );
  }

  return (
    <div className="category-breakdown">
      <div className="category-donut">
        <div className="category-donut-total">
          <span>Total de {label}</span>
          <strong title={format(data.total ?? "0")} style={{ fontSize: donutFigureSize(format(data.total ?? "0")) }}>
            {format(data.total ?? "0")}
          </strong>
        </div>
        <ResponsiveContainer width="100%" height={200}>
          <PieChart>
            <Pie
              data={slices}
              dataKey="value"
              nameKey="name"
              innerRadius="72%"
              outerRadius="95%"
              isAnimationActive={false}
              stroke="#fff"
              strokeWidth={2}
              onMouseEnter={(_, index) => setActive(slices[index].key)}
              onMouseLeave={() => setActive(null)}
              onClick={(_, index) => navigate(linkFor(slices[index]))}
            >
              {slices.map((slice) => (
                <Cell
                  key={slice.key}
                  fill={slice.color}
                  opacity={active === null || active === slice.key ? 1 : 0.35}
                  style={{ cursor: "pointer" }}
                />
              ))}
            </Pie>
            <Tooltip
              content={({ active: visible, payload }) => {
                const slice = payload?.[0]?.payload as Slice | undefined;
                return visible && slice ? (
                  <div className="timeline-chart-tooltip">
                    <strong>{slice.name}</strong>
                    <div>
                      {format(slice.amount)} · {percent(slice.value, total)}
                    </div>
                  </div>
                ) : null;
              }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      <ul className="category-breakdown-list" aria-label={`Categorias de ${label}`}>
        {slices.map((slice) => (
          <li key={slice.key} data-active={active === slice.key}>
            <Link
              to={linkFor(slice)}
              onFocus={() => setActive(slice.key)}
              onBlur={() => setActive(null)}
              onMouseEnter={() => setActive(slice.key)}
              onMouseLeave={() => setActive(null)}
            >
              <span className="category-breakdown-swatch" style={{ background: slice.color }} aria-hidden="true" />
              <span className="category-breakdown-name">{slice.name}</span>
              <span className="category-breakdown-amount">
                <strong>{format(slice.amount)}</strong>
                <span>{percent(slice.value, total)}</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function SpendingByCategoryCard({ period }: { period: Period }) {
  const [direction, setDirection] = useState<CategoryDirection>("outflow");
  const { data, isLoading, error, refetch } = useCategoryBreakdown(period, direction);

  return (
    <Section
      title="Por categoria"
      trailing={
        <Link className="touch-link" to={transactionLink(period, direction)}>
          Ver transações
        </Link>
      }
    >
      <div className="page-tabs">
        <PageTabs
          label="Tipo de movimentação"
          value={direction}
          onChange={setDirection}
          options={[
            { value: "outflow", label: "Saídas" },
            { value: "inflow", label: "Entradas" },
          ]}
        />
      </div>
      <div aria-live="polite" aria-busy={isLoading}>
        {isLoading ? (
          <CategorySkeleton />
        ) : error || !data ? (
          <UnavailableState onRetry={() => void refetch()}>Não foi possível carregar as categorias.</UnavailableState>
        ) : data.total === null ? (
          <div role="status"><p>{data.reporting ? reportingLabel(data.reporting) : "Total indisponível"}</p><ReportingDetails report={data.reporting} totals={data.totals} /></div>
        ) : (
          <ReportValuation valuation={data}><ReportingDetails report={data.reporting} totals={data.totals} /><Breakdown key={`${period.from}-${period.to}-${direction}`} data={data} period={period} direction={direction} /></ReportValuation>
        )}
      </div>
    </Section>
  );
}
