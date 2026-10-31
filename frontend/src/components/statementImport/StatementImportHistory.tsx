import { Link } from "react-router-dom";

import type { ImportHistoryItem } from "../../api/statementImports";
import { useTimezone } from "../../hooks/useTimezone";
import { formatOptionalDateTime } from "../../presentation/dates";

/**
 * Past imports as flat rows: account and file on the left, what the file
 * contained beside a link to the account. Like the sync history it lists
 * runs, but as rows so a phone never scrolls it sideways.
 */
export function StatementImportHistory({ items }: { items: ImportHistoryItem[] }) {
  const timezone = useTimezone();
  return (
    <ul className="statement-history-list" aria-label="Extratos importados">
      {items.map((item) => (
        <li key={item.run_id} className="statement-history-row">
          <div className="statement-history-identity">
            <span className="statement-history-account">{item.account_name}</span>
            <span className="statement-history-meta">{item.filename} · {formatOptionalDateTime(item.created_at, timezone)}</span>
          </div>
          <span className="statement-history-counts">
            {item.counts.new} novos · {item.counts.duplicate} já importados · {item.counts.invalid} inválidos
          </span>
          <Link
            className="statement-history-link"
            aria-label={`Ver conta ${item.account_name}`}
            to={`/contas-e-cartoes/${item.account_id}`}
          >
            Ver conta
          </Link>
        </li>
      ))}
    </ul>
  );
}
