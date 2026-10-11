# Investimentos

Contas de investimento representam a custódia; posições representam os ativos;
carteiras por objetivo agrupam posições sem criar um segundo saldo. Cada
posição pertence a uma única carteira ou fica sem objetivo.

## Domínio e APIs

`internal/investments` concentra contas, carteiras, posições manuais, operações,
avaliações e conciliação. As APIs usam `/api/investment-accounts`,
`/api/investment-portfolios`, `/api/investment-positions`,
`/api/investment-operations`, `/api/investment-reconciliations` e
`/api/investment-summary`. As consultas legadas `/api/investments` continuam
disponíveis para os ativos sincronizados e seu histórico importado.

Investimentos integrados mantêm os identificadores do provedor, agrupados
inicialmente pela conexão. Objetivos e conciliações são escolhas locais. Um
caixa de corretora já presente em `financial_accounts` pode ser vinculado,
sem adicionar novamente seu saldo ao patrimônio.

Operações manuais usam reais e aritmética decimal. Compras incorporam custos
ao custo médio ponderado; vendas baixam custo proporcional. Correções
reprocessam o histórico e rejeitam caixa ou quantidade negativos. Avaliações
são manuais e datadas; quando indisponíveis, o custo é identificado como custo,
sem inventar uma cotação ou rentabilidade. O livro guarda o custo total e o
valor total avaliado; custo médio e cotação unitária são derivados só para
exibição, de modo que valor e ganho não sofrem deriva decimal. Em ativos com
cotação automática o valor vem da série de preços (`market_quote`), calculado
na leitura: o job de cotações não grava operações no livro.

## Transferências e relatórios

Aporte e resgate transferem patrimônio. Compra e venda trocam caixa de
investimento por posição ou vice-versa. Rendimento recebido, taxa e imposto
têm natureza própria. Saldo inicial não é receita; valorização não é aporte.

Conciliações associam parcelas de lançamentos bancários às operações e,
quando disponível, ao histórico importado do investimento. A soma das parcelas
não pode exceder o lançamento. Sugestões exigem confirmação; o rótulo
`Investments` informado pelo provedor não basta para classificar um aporte.

`transactions.Item.EffectiveMoney` mantém o valor integral para extrato e
caixa. `InvestmentTransferAmount` identifica a parcela conciliada como
aporte/resgate; `ReportableAmount` identifica o restante elegível nos totais.
Uma conciliação integral recebe motivo `investment_transfer` apenas se o
lançamento ainda estava nos totais. Ignorado, categoria de transferência e
demais motivos já atribuídos têm precedência e são mantidos.
Gastos por categoria usam o restante; a curva de caixa preserva o movimento
integral. Os agregados mensais distinguem aportes e resgates.

Editar ou excluir um lançamento bancário manual conciliado exige desfazer
os vínculos primeiro. Ignorar um lançamento conciliado desfaz seus vínculos
(a operação volta a ser reportada sozinha), e um lançamento ignorado não pode
ser conciliado. A conciliação não altera saldos importados. Lançamentos
bancários manuais mantêm a regra de não modificar o saldo informado pelo banco.

## Patrimônio e limites

O patrimônio soma posições sincronizadas uma única vez, posições manuais e
caixa manual de investimento ainda não representado numa conta financeira
vinculada. Carteiras por objetivo não entram novamente na soma. Não são
inventadas valorizações para snapshots históricos sem dados.

Esta versão registra operações; não executa ordens ou transferências no banco.
Não inclui apuração fiscal, posições vendidas, câmbio nas operações manuais ou
agendamento de aportes recorrentes. A cotação automática é opcional e está
descrita em [Cotação automática](#cotação-automática).

## Classificação e catálogo de ativos

O cadastro organiza o ativo por **classe financeira** (`asset_class`) e
**tipo de instrumento** (`asset_type`), independentemente da instituição ou
do serviço de cotação:

| Classe | Tipos disponíveis |
| --- | --- |
| Renda fixa (`fixed_income`) | Título público, CDB, RDB, LCI, LCA, debênture, CRI, CRA, poupança, fundo e ETF de renda fixa |
| Renda variável (`variable_income`) | Ação, unit, BDR de ação, FII, Fiagro, fundo e ETF de ações |
| Multimercado (`multimarket`) | Fundo multimercado |
| Cambial (`currency`) | Fundo cambial e moeda estrangeira |
| Criptoativos (`crypto`) | Criptomoeda, stablecoin, token e ETF de criptoativos |
| Outros (`other`) | COE, derivativo, ouro e ETF de commodities |

PGBL e VGBL podem ser classificados em renda fixa, renda variável ou
multimercado conforme a exposição do plano. Todas as classes permitem
descrever outro tipo. Tipos antigos ou importados são preservados; a migration
classifica descrições reconhecíveis e deixa fundos e ETFs de exposição
desconhecida em Outros para o usuário revisar.

A API fornece as opções em `GET /api/investment-asset-classification`, usado
pelo formulário. Tipos conhecidos incompatíveis com a classe são recusados.

O catálogo inicial é opcional e pode ser criado com:

```sh
go run ./cmd/julius seed investment-assets
# Usa JULIUS_DB; também aceita -db caminho-ou-DSN depois de investment-assets.
```

A seed insere 27 ativos conhecidos: ações brasileiras, uma unit, FIIs, ETFs
de ações/renda fixa/criptoativos, BDRs e BTC, ETH, SOL e USDC. É executada em
uma transação e pode ser repetida: preserva ativos existentes, seus nomes,
classes e preferências de cotação, inclusive códigos equivalentes como
`PETR4.SA`. Os ativos ficam disponíveis para seleção em novas posições mesmo
quando ainda não há nenhuma posição deles. Não são criados contas, saldos,
operações ou preços, e a seed não faz chamadas externas.

CDBs e títulos públicos são tipos disponíveis; cada emissão/vencimento deve
ser cadastrado como um instrumento próprio. O catálogo é uma conveniência de
cadastro, sem ranking ou recomendação de investimento.

Referências: [características dos investimentos (CVM)](https://www.gov.br/investidor/pt-br/investir/antes-de-investir/entenda-as-caracteristicas-dos-investimentos),
[classes de fundos (ANBIMA)](https://developers.anbima.com.br/en/documentacao/fundos/apis-de-fundos/),
[instrumentos negociados (B3, maio de 2026)](https://www.b3.com.br/pt_br/noticias/levantamento-realizado-no-datawise-aponta-acoes-fundos-imobiliarios-etfs-e-bdrs-mais-negociados-em-maio.htm),
[ETFs de diferentes exposições (B3)](https://b3.com.br/pt_br/noticias/b3-ultrapassa-marca-de-200-etfs-listados-e-estoque-financeiro-dobra-em-um-ano.htm),
[Ether](https://ethereum.org/developers/docs/intro-to-ether),
[Solana](https://solana.com/learn/introduction-to-solana-tokens) e
[USDC](https://www.circle.com/usdc).

## Cotação automática

Um ativo com **cotação automática** tem o preço das suas posições manuais
buscado automaticamente. O formulário oferece um botão liga/desliga e deriva
o **mercado** do tipo de instrumento, guardado em `quote_source`: `b3` (ações, units, FIIs, ETFs,
BDRs e mercado fracionário) ou `crypto` (criptomoedas, cotadas em reais).
`quote_symbol` é o símbolo canônico do instrumento naquele mercado, sem
formato de provedor. Quem cadastra informa só o **código** do ativo (`ticker`)
e escolhe classe/tipo; com `quote_symbol` vazio, o símbolo é derivado do código
ao salvar e gravado no ativo. Ele só precisa ser informado quando o código
cotado difere do código do ativo.

Um ETF de renda fixa ou de criptoativos negociado na B3 usa `quote_source=b3`
e mantém sua classe financeira. Para tipos sem cotação suportada, o formulário
orienta usar avaliações manuais ou valores da instituição integrada.

- **B3:** `petr4`, `PETR4.SA` e `BVMF:PETR4` viram `PETR4`.
- **Cripto:** `btc`, `BTC-USD` e `BTC/BRL` viram `BTC`.

Um código que não pode ser lido no formato do mercado é recusado ao salvar
(`400`), com o motivo.

A atualização é habilitada por padrão, diariamente às `19:00` em
`America/Sao_Paulo`. `enabled`, `time` e `timezone` ficam juntos na tabela
`settings` e são lidos pelo agendador sem reiniciar o processo. Alterar horário
ou fuso reagenda a próxima execução; desabilitar suspende também o backfill.
O lote de cotações do dia já em andamento pode terminar; a desabilitação é
conferida antes do estágio de histórico e dos próximos ciclos de trabalho.

Na subida e ao reabilitar, busca só preços de hoje e histórico que faltam;
reiniciar não repete consultas já persistidas. A execução diária atualiza os
preços do dia. A data das cotações continua sendo o dia brasileiro,
independentemente do fuso do agendamento. Mudanças locais pela API acordam o
agendador imediatamente; outras gravações são detectadas em até um minuto.

`JULIUS_QUOTES_SCHEDULE` (`HH:MM` ou `HH:MM Zona/IANA`) inicializa somente uma
configuração ainda não salva. Sem fuso usa `Local`; vazio usa o novo padrão.
Depois da primeira gravação, a configuração persistida prevalece integralmente,
inclusive quando desabilitada, e alterações no ambiente são ignoradas.

### Provedores

`internal/marketdata` tenta os provedores em ordem de prioridade, com fallback
automático. A ordem vem de `JULIUS_QUOTES_PROVIDERS` (nomes separados por
vírgula; a ordem é a prioridade e um provedor omitido fica desligado), por
padrão `yahoo,brapi,coingecko`. Um nome desconhecido impede a subida do
processo. Cada provedor atende só os mercados que conhece, então no padrão:

| Mercado | Principal | Fallback |
| --- | --- | --- |
| `b3` | Yahoo Finance (`PETR4.SA`) | brapi |
| `crypto` | Yahoo Finance (`BTC-USD` × câmbio `BRL=X`) | CoinGecko (pares em BRL) |

O Yahoo Finance é consultado pelo endpoint público de gráfico
(`query1.finance.yahoo.com/v8/finance/chart`, o mesmo usado pelo yfinance).
Ativo não encontrado, provedor indisponível, tempo esgotado ou limite de taxa
fazem o próximo provedor do mercado ser tentado. Só quando todos os provedores
do mercado falham por limite de taxa o ativo é adiado: fica para a próxima
execução e nada é marcado como consultado.

Cada preço gravado (`investment_asset_quotes.source`) registra o provedor que o
serviu (`yahoo`, `brapi` ou `coingecko`). A cotação do dia fica 10 minutos em cache em memória por
instrumento. Na CoinGecko, o código (`BTC`) é resolvido para o id da moeda
(`bitcoin`) pela busca dela (vale a moeda de maior market cap com aquele
código); essa resolução fica em memória durante a vida do processo e não é
gravada no ativo.

O token da brapi (`PUT /api/settings/quotes`, `brapi_token`) é opcional e só
importa quando a brapi é usada como fallback da B3.

Limitações conhecidas do Yahoo Finance:

- Não há pares de cripto em reais (`BTC-BRL` responde `404`): a cotação é o
  preço em dólar (`BTC-USD`) × o câmbio USD/BRL (`BRL=X`) do dia. Em dias sem
  câmbio (fins de semana), vale o último câmbio anterior.
- Os preços vêm em ponto flutuante e são arredondados às casas indicadas pelo
  próprio Yahoo (`priceHint`).
- É um endpoint público não documentado; quando ele falha, o fallback assume.

### Histórico

Uma posição registrada com data no passado precisa de preço nessas datas. O
backfill é guiado pelo que falta, não pelo que mudou: para cada ativo com
cotação automática ele compara o primeiro dia em que alguma posição manual o
teve com o intervalo já consultado (`investment_asset_quote_coverage`, um
intervalo por ativo) e pede aos provedores só o trecho que falta, até ontem (o
preço de hoje é da execução diária). Isso cobre uma posição nova com data
antiga, uma compra corrigida para uma data anterior, uma troca de mercado ou
símbolo e os dias de uma execução perdida. Gravar posição, operação ou ativo
acorda o agendador (com 2 s de espera para juntar uma rajada de gravações) e
preços de hoje que ainda faltam também são buscados; o que já está coberto não
gera requisição.

O intervalo é chaveado pelo instrumento (colunas `source` = mercado e `symbol`
= símbolo canônico), não pelo provedor: trocar a ordem dos provedores ou cair
no fallback não busca o histórico de novo; mudar o mercado ou o símbolo do
ativo, sim. O intervalo guardado é o que foi **pedido**, não o que foi obtido:
dias sem pregão ficam sem preço de propósito, e um trecho que o provedor não
serve não é pedido de novo a cada execução. A exceção é o histórico vindo de
um provedor de fallback que serviu menos do que foi pedido (por exemplo,
Yahoo fora do ar e plano da brapi com só 3 meses): aí só o trecho servido é
registrado, e os dias mais antigos são pedidos de novo numa próxima execução,
quando o provedor principal pode ter voltado.

| Provedor | Mercados (papel no padrão) | Histórico | Limite de taxa |
| --- | --- | --- | --- |
| Yahoo Finance | `b3` e `crypto` (principal) | completo, sem limite de período | sem número publicado; o app usa 1 requisição por segundo, serializadas, e respeita `429`/`Retry-After` |
| brapi sem token | `b3` (fallback) | só tickers PETR4, MGLU3, VALE3, ITUB4 | 20 requisições por minuto por IP (`RateLimit-*`); o app usa 1 a cada 3,2 s |
| brapi com token | `b3` (fallback) | conforme o plano: 3 meses (Free), 1 ano (Startup), mais de 10 anos (Pro) | cota mensal e concorrência por conta (1 no Free); o app serializa as requisições |
| CoinGecko (API pública) | `crypto` (fallback) | até 365 dias atrás, 1 ponto por dia (00:00 UTC) | por IP, sem número publicado; o app usa 1 requisição a cada 6 s e respeita `Retry-After` |

A brapi só aceita períodos relativos (`range`), então o app pede o menor que
alcança o primeiro dia; se o plano recusar, tenta períodos menores. O preço de
um dia passado é o fechamento ajustado por desdobramentos e grupamentos, nunca
o ajustado por proventos: o `close` do Yahoo e o da brapi seguem o mesmo
critério.

Cada provedor tem um limitador compartilhado por todo o processo (execução
diária, backfill e resolução de código da CoinGecko). Ele espaça as
requisições, serializa-as e pausa o provedor quando ele pede (`429` com
`Retry-After`, ou janela `RateLimit-Remaining: 0` até `RateLimit-Reset`). Uma
pausa longa não é esperada com a requisição aberta: o provedor pausado é pulado
e o próximo do mercado é tentado.

### Origem e precedência dos preços

Cada linha da série (`investment_asset_quotes`, um preço por ativo por dia) tem
`source`, quem a gravou (`pluggy`, `issue`, `yahoo`, `brapi`, `coingecko`), só
para exibição e auditoria, e `origin`, o tipo de afirmação que o preço faz,
que é o que decide quem pode substituir quem:

| `origin` | O que é | Quem grava | Quem pode substituí-lo |
| --- | --- | --- | --- |
| `sync` | preço que o Open Finance informou na sincronização | `UpsertAssetQuote` (snapshot da Pluggy, inclusive o refeito dos `raw_imports`) | outra sincronização |
| `issue` | PU de emissão de um título de renda fixa, derivado da compra | `insertAssetQuoteIfAbsent` | só uma sincronização; nunca preenche um dia já preenchido |
| `market` | cotação (spot ou fechamento) de um provedor de mercado | `UpsertConnectorQuotes` (execução diária e backfill) | outro preço `market`, de qualquer provedor, ou uma sincronização |

A migration classifica as linhas existentes pelo `source` (`pluggy` vira `sync`,
`issue` vira `issue`, o restante vira `market`). Os escritores recusam `origin`
vazio ou desconhecido (`ErrInvalidInput`), e `UpsertConnectorQuotes` só aceita
`market`. O job considera um ativo já cotado no dia (`RefreshMissing`) só quando
há preço `market` de hoje: o preço que a sincronização informou não evita a
consulta ao mercado, que por sua vez não o substitui.

### Rendimento

`investment_asset_quotes` (a série de preço diário do ativo) é a **única fonte
de preço**: o Open Finance grava nela a cada sincronização, a execução diária
do job de cotações grava o preço do dia dos ativos com cotação automática que
alguma posição manual mantém com quantidade positiva, e o backfill grava o
histórico. O job **não grava operações** `valuation` no livro: o preço do dia só
entra na série.

O valor de uma posição manual cujo ativo tem cotação automática é **derivado na
leitura** (`Position.CurrentValue`, de onde saem totais, metas e patrimônio):
`quantidade × último preço da série`, arredondado a centavos, com
`valuation_basis = market_quote`, `current_unit_price` igual ao preço e
`valued_on` igual ao dia dele. Nada disso é persistido; uma cotação velha
continua valendo para o valor exibido, e `valued_on` mostra a idade dela.

A regra é "a afirmação mais recente sobre o preço vence; no empate de dia, a
avaliação digitada vence": a cotação só vale se a posição tem quantidade
positiva e não há avaliação digitada, ou a última avaliação é de um dia anterior
ao da cotação. Do contrário vale a avaliação (`manual_valuation`), que
continua valendo até chegar uma cotação mais nova. Ativos sem cotação
automática seguem avaliados só pelas avaliações do livro (ou pelo custo, com
`cost_basis`).

O rendimento usa a mesma regra em qualquer dia: uma posição manual cujo ativo
tem cotação automática é avaliada por `quantidade mantida no fim do dia × preço
do dia` (último preço até 10 dias antes, arredondado a centavos como o de uma
posição sincronizada), a menos que a última avaliação digitada até aquele dia
seja igual ou mais recente que esse preço. Assim o rendimento de qualquer
período dentro do histórico não depende de existir uma avaliação datada nas
pontas. A série não grava nada derivado: o valor histórico é sempre calculado
na leitura.

O teto de 10 dias vale só para o rendimento. O valor atual da posição usa a
última cotação de qualquer idade (`valued_on` mostra de que dia ela é), para o
valor exibido não saltar numa queda de provedor. Quando a última cotação é mais
velha que 10 dias e mais nova que a última avaliação digitada, o rendimento
daquele dia fica indisponível (`saldo_indisponivel`): não volta a uma avaliação
que a cotação já substituiu.

## Ações combinadas, revisão e filtros

`POST /api/investment-operations/batch` recebe `operations` (de 1 a 100 itens)
com o mesmo formato de uma operação, e `reconciliation` opcional. O vínculo
refere-se à primeira operação do lote. A gravação inteira é transacional:
uma compra inválida ou vínculo recusado não deixa aporte ou rendimento órfão.
A interface oferece aporte com compra e rendimento reinvestido. Quantidade e
preço calculam o valor bruto em decimal quando `amount` não é informado.

Contas integradas aceitam registros locais de aporte, resgate, rendimento,
taxa e imposto. Esses registros alimentam relatórios e conciliação; nunca
recalculam saldos ou posições do provedor. Compra, venda, saldo inicial e
cotação de posições integradas continuam sendo dados da instituição.

Um lançamento bancário também pode ser conciliado diretamente com um movimento
importado de conta integrada, sem registro manual: `POST
/api/investment-reconciliations` sem `operation_id` deriva uma operação
`source = synced` na custódia da conexão (aporte para `inflow`, rendimento para
`INTEREST`, resgate para os demais `outflow`), com valor e data do movimento.
A operação derivada é somente leitura, é reaproveitada por parcelas seguintes
do mesmo movimento (inclusive depois de desfeita a parcela que registrou o
movimento), some ao desfazer o último vínculo e nunca entra nos relatórios
por conta própria. Movimento sem direção, sem data ou fora de BRL
é recusado.

Uma transferência entre custódias leva o custo médio que a posição de origem
tinha na data da transferência; esse custo fica gravado na entrada de destino
e correções posteriores às compras de origem não o propagam.
Uma posição manual pode começar vazia para receber sua primeira compra.
A última cotação manual por unidade continua válida após compras e vendas;
a quantidade remanescente é avaliada por essa cotação, mantendo sua data.
Correções reprocessam o histórico na ordem de data, criação e identificador.

A conciliação confere direção, moeda BRL e capacidade restante de cada
lançamento, operação e movimento importado. Para contas integradas, o
movimento importado deve pertencer à mesma conexão. Parcelas concorrentes
são serializadas durante a validação. Sugestões na interface usam igualdade
do valor disponível e até cinco dias de diferença; exigem confirmação.
Taxas, impostos e rendimentos também podem ser vinculados, evitando duplicar
receitas e despesas. Desvincular restaura a parcela disponível.

A tela Investimentos oferece “Revisar lançamentos antigos”, abrindo o extrato
sem restrição de período. `POST /api/transactions/query` aceita
`filters.origin` (`manual`, `synced`). Nenhum histórico é reclassificado
automaticamente.

Posições aceitam filtros `account_id`, `portfolio_id`, `source`, `source_id`
e `include_closed`. Operações aceitam `account_id`, `position_id`,
`portfolio_id`, `source`, `source_id`, `from`, `to` e `reconciliation_state`
(`linked`, `unlinked`). As avaliações digitadas são operações `valuation`
datadas, consultáveis pelo mesmo histórico de operações; o job de cotações
nunca as cria.

O total da área de investimentos inclui o caixa efetivo das contas vinculadas.
No patrimônio esse mesmo caixa já está na parcela bancária, portanto não é
somado uma segunda vez. Na visão por objetivo, compras e vendas são rotuladas
como tal, sem confundi-las com as transferências de caixa da custódia.

Posições importadas em outras moedas mantêm a moeda e o valor originais.
Sem conversão cambial nesta versão, ficam fora dos totais de investimento,
metas e parcela de investimentos do patrimônio em reais. A interface sinaliza
quando existem posições fora desses totais.
