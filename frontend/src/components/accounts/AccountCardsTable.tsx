import { Alert, Skeleton } from "../ui";

import type { AccountCard } from "../../api/contracts";
import { formatOptionalLocalDay } from "../../presentation/dates";
import { maskedCardNumber } from "../../presentation/accountLabels";
import { EmptyState, ListRow, Section } from "../layout";

/** The physical cards spending against one credit account — several PANs can
 *  share the same limit and invoice — as flat rows: the card number, then one
 *  quiet line with how many transactions it made and when it was last used.
 *  The account is a single record; this is the only place the individual card
 *  numbers show up, since they live in transaction metadata rather than in
 *  an entity of their own. */
export function AccountCardsTable({
  cards,
  isLoading,
  error,
}: {
  cards: AccountCard[];
  isLoading: boolean;
  error: unknown;
}) {
  // One physical card is a fact about the account, not a list: a titled
  // section for a single row is more frame than content, so it is one quiet row.
  if (cards.length === 1 && !isLoading && (error === null || error === undefined)) {
    const [card] = cards;
    return (
      <section className="section accounts-single-card" aria-label="Cartão vinculado">
        <ul className="list-rows">
          <ListRow
            title={`Cartão ${maskedCardNumber(card.card_number)}`}
            meta={`${card.transaction_count} ${card.transaction_count === 1 ? "transação" : "transações"} · Último uso ${formatOptionalLocalDay(card.last_transaction_at)}`}
          />
        </ul>
      </section>
    );
  }

  return (
    <Section title="Cartões vinculados">
      {error !== null && error !== undefined && (
        <Alert type="error" showIcon message="Não foi possível carregar os cartões desta conta." />
      )}
      {isLoading ? (
        <div role="status" aria-label="Carregando cartões vinculados">
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : cards.length === 0 ? (
        <EmptyState
          title="Nenhum cartão vinculado"
          hint="Os números dos cartões aparecem aqui quando surgem nas transações desta conta."
        />
      ) : (
        <ul className="list-rows" aria-label="Cartões vinculados">
          {cards.map((card) => (
            <ListRow
              key={card.card_number}
              title={maskedCardNumber(card.card_number)}
              meta={`${card.transaction_count} ${card.transaction_count === 1 ? "transação" : "transações"} · Último uso ${formatOptionalLocalDay(card.last_transaction_at)}`}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
