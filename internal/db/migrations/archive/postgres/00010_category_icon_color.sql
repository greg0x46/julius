-- +goose Up

ALTER TABLE categories ADD COLUMN icon TEXT NOT NULL DEFAULT 'ellipsis';
ALTER TABLE categories ADD COLUMN color TEXT NOT NULL DEFAULT '#6c757d';

UPDATE categories SET icon = 'shopping-cart',        color = '#2a78d6' WHERE id = '000433b6-3094-5a9c-87df-465b70574a4b'; -- Supermercado
UPDATE categories SET icon = 'coffee',               color = '#eb6834' WHERE id = '12cdb9e7-3f28-5fcf-a675-a2195a732bf1'; -- Alimentação
UPDATE categories SET icon = 'car',                  color = '#17a2b8' WHERE id = '99f20e22-cd96-5081-afd0-068a203c5fde'; -- Transporte
UPDATE categories SET icon = 'shopping',              color = '#e64980' WHERE id = '11d50e25-577d-5780-bbae-35a6b14c7d01'; -- Compras
UPDATE categories SET icon = 'bank',                  color = '#d64545' WHERE id = '3b734076-b597-545b-b302-47061c2278bf'; -- Empréstimos
UPDATE categories SET icon = 'home',                  color = '#b8860b' WHERE id = 'bbdc84e0-788a-59f7-a9f4-b2143172e53b'; -- Moradia
UPDATE categories SET icon = 'smile',                 color = '#eda100' WHERE id = '659c118e-42ad-5427-898e-cea0153525ed'; -- Lazer
UPDATE categories SET icon = 'percentage',            color = '#495057' WHERE id = '2231c10d-ff72-59af-9513-72516f9f452e'; -- Impostos e taxas
UPDATE categories SET icon = 'medicine',              color = '#37b24d' WHERE id = '17f56080-837c-5ebb-a828-93fcb891bb8f'; -- Saúde
UPDATE categories SET icon = 'scissor',                color = '#e87ba4' WHERE id = '7f185706-e8f2-5c1c-893a-150a93cfa133'; -- Cuidados pessoais
UPDATE categories SET icon = 'team',                  color = '#4c6ef5' WHERE id = '15278c27-bfce-5ab7-bbe5-eb7351425226'; -- Família
UPDATE categories SET icon = 'tool',                  color = '#6c757d' WHERE id = 'aec4034c-f5a2-59a0-a1f9-1d737e4f1f3f'; -- Serviços
UPDATE categories SET icon = 'wifi',                  color = '#099268' WHERE id = '1dddd1b3-1a1e-5c05-9a75-6359c68ce466'; -- Serviços digitais
UPDATE categories SET icon = 'book',                  color = '#7c5cbf' WHERE id = '51ed2111-26c8-5a0f-bffb-42aa00b5a0ce'; -- Educação
UPDATE categories SET icon = 'safety-certificate',    color = '#1baf7a' WHERE id = '223e55fe-b31e-539d-83aa-0b22427fc17f'; -- Seguros
UPDATE categories SET icon = 'heart',                 color = '#f76707' WHERE id = '6c5acb3a-477c-5ea0-a3f0-a036298d603a'; -- Pets
UPDATE categories SET icon = 'global',                color = '#2a78d6' WHERE id = 'a5318bcc-de1c-52c3-889b-2142e9c02408'; -- Viagens
UPDATE categories SET icon = 'gift',                  color = '#e87ba4' WHERE id = 'a5632bd8-442b-5a7c-9375-f05a2550f73f'; -- Presentes e doações
UPDATE categories SET icon = 'line-chart',            color = '#099268' WHERE id = 'b70b2537-8d98-55f3-930a-fd5d9b40cb2b'; -- Investimentos
UPDATE categories SET icon = 'ellipsis',              color = '#6c757d' WHERE id = 'b74c367a-9409-516c-b548-00a7207f67bd'; -- Outros
UPDATE categories SET icon = 'money-collect',         color = '#1baf7a' WHERE id = '3c5a9586-2a11-556d-b014-692ed51c3997'; -- Salário
UPDATE categories SET icon = 'laptop',                color = '#4c6ef5' WHERE id = 'c814e5a7-d199-5455-93ef-4c6369ce0e0a'; -- Trabalho autônomo
UPDATE categories SET icon = 'trophy',                color = '#eda100' WHERE id = 'c86337c1-3771-5917-aa53-5540e8d937bb'; -- Benefícios
UPDATE categories SET icon = 'rise',                  color = '#37b24d' WHERE id = '78bc85d7-21fc-5965-908f-a707208e0b1b'; -- Rendimentos
UPDATE categories SET icon = 'rollback',              color = '#17a2b8' WHERE id = 'be4336bf-2090-58cd-babe-c7755e035619'; -- Reembolsos
UPDATE categories SET icon = 'wallet',                color = '#b8860b' WHERE id = 'd7ea08b8-b80b-5e93-8352-9354ac7fe6d8'; -- Empréstimos recebidos
UPDATE categories SET icon = 'plus-circle',           color = '#6c757d' WHERE id = '6885e5af-bcc8-5dec-b6a2-45033d62376b'; -- Outras receitas
UPDATE categories SET icon = 'swap',                  color = '#7c5cbf' WHERE id = '533d9187-99b6-542b-a2f3-6eb9cbb299ce'; -- Transferência entre Contas Próprias

-- +goose Down

ALTER TABLE categories DROP COLUMN color;
ALTER TABLE categories DROP COLUMN icon;
