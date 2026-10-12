-- +goose Up

-- Pagamento de fatura de cartão de crédito: as duas pernas de pagar sua
-- própria fatura com sua própria conta (o débito no banco e o crédito no
-- cartão) são dinheiro real que se moveu, mas entre contas do próprio
-- usuário — não é receita nem despesa. kind=transfer dá o mesmo tratamento
-- de money.Eligibility que SamePersonTransferLabel já tem
-- (00004_categories.sql): Included=false, MovesCash=true (ver
-- money.MovedCash's ReasonTransferCategory). A categorização automática que
-- atribui isso está em internal/categories/cardpayment.go.
INSERT INTO categories (id, name, kind, is_active, icon, color, created_at, updated_at) VALUES
    ('dd10c680-fb35-4457-8595-4e51c8d279a7', 'Pagamento de Fatura de Cartão', 'transfer', 1, 'credit-card', '#7c5cbf', '2026-09-03T00:00:00.000000000Z', '2026-09-03T00:00:00.000000000Z');

-- +goose Down

DELETE FROM categories WHERE id = 'dd10c680-fb35-4457-8595-4e51c8d279a7';
