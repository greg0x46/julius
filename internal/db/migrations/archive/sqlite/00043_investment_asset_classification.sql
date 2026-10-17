-- Financial classification is independent from custody and quote routing.
-- Preserve existing type descriptions and classify only recognizable labels.
-- +goose Up
ALTER TABLE investment_assets ADD COLUMN asset_class TEXT NOT NULL DEFAULT 'other'
    CHECK (asset_class IN ('fixed_income', 'variable_income', 'multimarket', 'currency', 'crypto', 'other'));

UPDATE investment_assets SET asset_class = CASE
    WHEN lower(trim(asset_type)) IN ('título público', 'cdb', 'rdb', 'lci', 'lca', 'debênture', 'cri', 'cra', 'poupança', 'fundo de renda fixa', 'etf de renda fixa', 'fixed_income', 'renda fixa', 'tesouro', 'tesouro direto', 'government_bond', 'corporate_bond', 'debenture', 'debentures', 'Título público', 'Cdb', 'Rdb', 'Lci', 'Lca', 'Debênture', 'Cri', 'Cra', 'Poupança', 'Fundo de renda fixa', 'Etf de renda fixa', 'Fixed_income', 'Renda fixa', 'Tesouro', 'Tesouro direto', 'Government_bond', 'Corporate_bond', 'Debenture', 'Debentures', 'tÍtulo pÚblico', 'debÊnture', 'poupanÇa') THEN 'fixed_income'
    WHEN lower(trim(asset_type)) IN ('ação', 'unit', 'bdr de ação', 'fii', 'fiagro', 'etf de ações', 'fundo de ações', 'equity', 'stock', 'stocks', 'ações', 'acao', 'real_estate_fund', 'fundo imobiliário', 'bdr', 'renda variável', 'renda variavel', 'Ação', 'Unit', 'Bdr de ação', 'Fii', 'Fiagro', 'Etf de ações', 'Fundo de ações', 'Equity', 'Stock', 'Stocks', 'Ações', 'Acao', 'Real_estate_fund', 'Fundo imobiliário', 'Bdr', 'Renda variável', 'Renda variavel', 'aÇÃo', 'bdr de aÇÃo', 'etf de aÇÕes', 'fundo de aÇÕes', 'aÇÕes', 'fundo imobiliÁrio', 'renda variÁvel') THEN 'variable_income'
    WHEN lower(trim(asset_type)) IN ('fundo multimercado', 'multimercado', 'multimarket', 'Fundo multimercado', 'Multimercado', 'Multimarket') THEN 'multimarket'
    WHEN lower(trim(asset_type)) IN ('fundo cambial', 'moeda estrangeira', 'cambial', 'currency', 'Fundo cambial', 'Moeda estrangeira', 'Cambial', 'Currency') THEN 'currency'
    WHEN lower(trim(asset_type)) IN ('criptomoeda', 'stablecoin', 'token', 'etf de criptoativos', 'criptoativo', 'criptoativos', 'crypto', 'cryptocurrency', 'Criptomoeda', 'Stablecoin', 'Token', 'Etf de criptoativos', 'Criptoativo', 'Criptoativos', 'Crypto', 'Cryptocurrency') THEN 'crypto'
    ELSE 'other'
END;

-- +goose Down
ALTER TABLE investment_assets DROP COLUMN asset_class;
