import { useTranslation } from "../../i18n/context";
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
  const { t, language } = useTranslation();
  const cardMeta = (card: AccountCard) =>
    t("cardTransactionsLastUse", {
      count: card.transaction_count,
      unit: card.transaction_count === 1 ? t("transactionSingularLower") : t("transactionPluralLower"),
      date: formatOptionalLocalDay(card.last_transaction_at, language),
    });
  // One physical card is a fact about the account, not a list: a titled
  // section for a single row is more frame than content, so it is one quiet row.
  if (cards.length === 1 && !isLoading && (error === null || error === undefined)) {
    const [card] = cards;
    return (
      <section className="section accounts-single-card" aria-label={t("linkedCard")}>
        <ul className="list-rows">
          <ListRow
            title={t("cardMasked", { number: maskedCardNumber(card.card_number) })}
            meta={cardMeta(card)}
          />
        </ul>
      </section>
    );
  }

  const failed = error !== null && error !== undefined;
  return (
    <Section title={t("linkedCards")}>
      {failed && (
        <Alert type="error" showIcon message={t("cardsLoadError")} />
      )}
      {isLoading ? (
        <div role="status" aria-label={t("loadingLinkedCards")}>
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : cards.length === 0 ? (
        failed ? null : (
          <EmptyState
            title={t("noLinkedCard")}
            hint={t("noLinkedCardHint")}
          />
        )
      ) : (
        <ul className="list-rows" aria-label={t("linkedCards")}>
          {cards.map((card) => (
            <ListRow
              key={card.card_number}
              title={maskedCardNumber(card.card_number)}
              meta={cardMeta(card)}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
