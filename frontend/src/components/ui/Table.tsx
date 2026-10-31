import { useTranslation } from "../../i18n/context";
import {
  Fragment,
  useState,
  type CSSProperties,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { Button } from "./controls";
export interface Column<T> {
  className?: string;
  title?: ReactNode;
  dataIndex?: keyof T | string | string[];
  key?: string;
  render?: (value: unknown, record: T, index: number) => ReactNode;
  align?: "left" | "right" | "center";
  width?: number | string;
  filters?: { text: string; value: string | number | boolean }[];
  onFilter?: (value: string | number | boolean, record: T) => boolean;
  sorter?: (a: T, b: T) => number;
}
export type ColumnsType<T> = Column<T>[];
interface TableProps<T> {
  columns: Column<T>[];
  dataSource: T[];
  rowKey: keyof T | ((record: T) => string);
  onRow?: (record: T) => HTMLAttributes<HTMLTableRowElement>;
  pagination?: false | { pageSize?: number };
  loading?: boolean;
  expandable?: {
    expandRowByClick?: boolean;
    columnTitle?: ReactNode;
    expandedRowRender: (record: T) => ReactNode;
    rowExpandable?: (record: T) => boolean;
    expandedRowKeys?: readonly string[];
    onExpandedRowsChange?: (keys: readonly string[]) => void;
  };
  locale?: { emptyText?: ReactNode };
  size?: string;
  scroll?: { x?: number | string };
  style?: CSSProperties;
  className?: string;
  "aria-label"?: string;
}
function cellValue<T>(row: T, path: Column<T>["dataIndex"]): unknown {
  if (!path) return undefined;
  return (Array.isArray(path) ? path : [path]).reduce<unknown>(
    (value, key) =>
      typeof value === "object" && value !== null
        ? (value as Record<string, unknown>)[String(key)]
        : undefined,
    row,
  );
}
export default function Table<T>({
  columns,
  dataSource,
  rowKey,
  onRow,
  pagination = false,
  expandable,
  loading,
  locale,
  className,
  style,
  "aria-label": label,
}: TableProps<T>) {
  const { t } = useTranslation();
  const [filters, setFilters] = useState<Record<number, string>>({});
  const [sort, setSort] = useState<{ index: number; direction: number } | null>(
    null,
  );
  const [page, setPage] = useState(1);
  const [expanded, setExpanded] = useState<readonly string[]>([]);
  const keys = expandable?.expandedRowKeys ?? expanded;
  let rows = dataSource.filter((row) =>
    columns.every(
      (column, index) =>
        !filters[index] ||
        !column.onFilter ||
        column.onFilter(
          column.filters?.find((item) => String(item.value) === filters[index])
            ?.value ?? filters[index],
          row,
        ),
    ),
  );
  if (sort && columns[sort.index]?.sorter)
    rows = [...rows].sort(
      (a, b) => columns[sort.index].sorter!(a, b) * sort.direction,
    );
  const pageSize = pagination
    ? (pagination.pageSize ?? 20)
    : Math.max(rows.length, 1);
  const currentPage = Math.min(
    page,
    Math.max(1, Math.ceil(rows.length / pageSize)),
  );
  const visible = rows.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );
  return (
    <div
      className={`ui-table-wrapper ${className ?? ""}`}
      style={style}
      aria-busy={loading}
    >
      <div className="ui-table-scroll">
        <table className="ui-table" aria-label={label}>
          <thead>
            <tr>
              {expandable && (
                <th scope="col">
                  {expandable.columnTitle ?? (
                    <span className="visually-hidden">{t("details")}</span>
                  )}
                </th>
              )}
              {columns.map((column, index) => (
                <th
                  key={column.key ?? index}
                  scope="col"
                  style={{ textAlign: column.align, width: column.width }}
                  aria-sort={
                    sort?.index === index
                      ? sort.direction === 1
                        ? "ascending"
                        : "descending"
                      : undefined
                  }
                >
                  {column.sorter ? (
                    <button
                      type="button"
                      onClick={() =>
                        setSort({
                          index,
                          direction:
                            sort?.index === index ? -sort.direction : 1,
                        })
                      }
                    >
                      {column.title}
                    </button>
                  ) : (
                    column.title
                  )}
                  {column.filters && (
                    <label className="ui-table-filter">
                      <span className="visually-hidden">
                        Filtrar por{" "}
                        {typeof column.title === "string"
                          ? column.title.toLocaleLowerCase("pt-BR")
                          : "valor"}
                      </span>
                      <select
                        value={filters[index] ?? ""}
                        onChange={(event) => {
                          setFilters({
                            ...filters,
                            [index]: event.target.value,
                          });
                          setPage(1);
                        }}
                      >
                        <option value="">{t("all")}</option>
                        {column.filters.map((item) => (
                          <option
                            key={String(item.value)}
                            value={String(item.value)}
                          >
                            {item.text}
                          </option>
                        ))}
                      </select>
                    </label>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visible.map((row, index) => {
              const key = String(
                typeof rowKey === "function" ? rowKey(row) : row[rowKey],
              );
              const canExpand =
                expandable && (expandable.rowExpandable?.(row) ?? true);
              return (
                <Fragment key={key}>
                  <tr
                    {...onRow?.(row)}
                    onClick={(event) => {
                      onRow?.(row).onClick?.(event);
                      if (
                        canExpand &&
                        expandable.expandRowByClick &&
                        !(event.target as HTMLElement).closest(
                          "button, a, input, select",
                        )
                      ) {
                        const next = keys.includes(key)
                          ? keys.filter((item) => item !== key)
                          : [...keys, key];
                        setExpanded(next);
                        expandable.onExpandedRowsChange?.(next);
                      }
                    }}
                  >
                    {expandable && (
                      <td>
                        {canExpand && (
                          <Button
                            type="text"
                            aria-label={
                              keys.includes(key)
                                ? t("collapseDetails")
                                : t("expandDetails")
                            }
                            aria-expanded={keys.includes(key)}
                            onClick={() => {
                              const next = keys.includes(key)
                                ? keys.filter((item) => item !== key)
                                : [...keys, key];
                              setExpanded(next);
                              expandable.onExpandedRowsChange?.(next);
                            }}
                          >
                            {keys.includes(key) ? "−" : "+"}
                          </Button>
                        )}
                      </td>
                    )}
                    {columns.map((column, ci) => {
                      const value = cellValue(row, column.dataIndex);
                      return (
                        <td
                          key={column.key ?? ci}
                          className={column.className}
                          style={{ textAlign: column.align }}
                        >
                          {column.render
                            ? column.render(value, row, index)
                            : value == null
                              ? ""
                              : String(value)}
                        </td>
                      );
                    })}
                  </tr>
                  {canExpand && keys.includes(key) && (
                    <tr>
                      <td colSpan={columns.length + 1}>
                        {expandable.expandedRowRender(row)}
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
            {!visible.length && (
              <tr>
                <td colSpan={columns.length + (expandable ? 1 : 0)}>
                  {loading
                    ? t("loading")
                    : (locale?.emptyText ?? t("noRecords"))}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {pagination && rows.length > pageSize && (
        <nav aria-label={t("pagination")} className="ui-pagination">
          <Button
            disabled={currentPage === 1}
            onClick={() => setPage(currentPage - 1)}
          >
            {t("previous")}
          </Button>
          <span>
            {currentPage} / {Math.ceil(rows.length / pageSize)}
          </span>
          <Button
            disabled={currentPage * pageSize >= rows.length}
            onClick={() => setPage(currentPage + 1)}
          >
            {t("next")}
          </Button>
        </nav>
      )}
    </div>
  );
}
