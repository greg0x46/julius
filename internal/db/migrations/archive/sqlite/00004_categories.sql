-- +goose Up

CREATE TABLE categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('expense', 'income', 'transfer')),
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Deterministic uuid5 values (namespace `contadinho.categories`), kept
-- identical to the Python reference's app/categories/constants.py so category
-- identity is stable across the two implementations.
INSERT INTO categories (id, name, kind, is_active, created_at, updated_at) VALUES
    ('000433b6-3094-5a9c-87df-465b70574a4b', 'Supermercado', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('12cdb9e7-3f28-5fcf-a675-a2195a732bf1', 'Alimentação', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('99f20e22-cd96-5081-afd0-068a203c5fde', 'Transporte', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('11d50e25-577d-5780-bbae-35a6b14c7d01', 'Compras', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('3b734076-b597-545b-b302-47061c2278bf', 'Empréstimos', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('bbdc84e0-788a-59f7-a9f4-b2143172e53b', 'Moradia', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('659c118e-42ad-5427-898e-cea0153525ed', 'Lazer', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('2231c10d-ff72-59af-9513-72516f9f452e', 'Impostos e taxas', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('17f56080-837c-5ebb-a828-93fcb891bb8f', 'Saúde', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('7f185706-e8f2-5c1c-893a-150a93cfa133', 'Cuidados pessoais', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('15278c27-bfce-5ab7-bbe5-eb7351425226', 'Família', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('aec4034c-f5a2-59a0-a1f9-1d737e4f1f3f', 'Serviços', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('1dddd1b3-1a1e-5c05-9a75-6359c68ce466', 'Serviços digitais', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('51ed2111-26c8-5a0f-bffb-42aa00b5a0ce', 'Educação', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('223e55fe-b31e-539d-83aa-0b22427fc17f', 'Seguros', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('6c5acb3a-477c-5ea0-a3f0-a036298d603a', 'Pets', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('a5318bcc-de1c-52c3-889b-2142e9c02408', 'Viagens', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('a5632bd8-442b-5a7c-9375-f05a2550f73f', 'Presentes e doações', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('b70b2537-8d98-55f3-930a-fd5d9b40cb2b', 'Investimentos', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('b74c367a-9409-516c-b548-00a7207f67bd', 'Outros', 'expense', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('3c5a9586-2a11-556d-b014-692ed51c3997', 'Salário', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('c814e5a7-d199-5455-93ef-4c6369ce0e0a', 'Trabalho autônomo', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('c86337c1-3771-5917-aa53-5540e8d937bb', 'Benefícios', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('78bc85d7-21fc-5965-908f-a707208e0b1b', 'Rendimentos', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('be4336bf-2090-58cd-babe-c7755e035619', 'Reembolsos', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('d7ea08b8-b80b-5e93-8352-9354ac7fe6d8', 'Empréstimos recebidos', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('6885e5af-bcc8-5dec-b6a2-45033d62376b', 'Outras receitas', 'income', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z'),
    ('533d9187-99b6-542b-a2f3-6eb9cbb299ce', 'Transferência entre Contas Próprias', 'transfer', 1, '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z');

CREATE TABLE transaction_category_decisions (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic'))
);

CREATE INDEX ix_transaction_category_decisions_category_id ON transaction_category_decisions (category_id);

CREATE TABLE transaction_category_events (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    -- No FK on previous/resulting_category_id: this table is append-only
    -- (see triggers below), mirroring rule_id in transaction_inclusion_events.
    previous_category_id TEXT,
    resulting_category_id TEXT NOT NULL,
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic')),
    UNIQUE (transaction_id, revision)
);

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER transaction_category_events_reject_delete;
DROP TRIGGER transaction_category_events_reject_update;
DROP TABLE transaction_category_events;
DROP TABLE transaction_category_decisions;
DROP TABLE categories;
