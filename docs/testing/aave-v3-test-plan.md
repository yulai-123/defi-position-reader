# Aave V3 测试计划

## 测试分层

1. Unit tests
   - ABI pack/unpack。
   - Multicall 编码和解码。
   - Metadata sync 的 reserve / vault 解析。
   - Fetcher 的 lending / yield 输出。
   - Metadata 缺失、过期、RPC failover 等错误路径。

2. Live smoke tests
   - 默认不在 `go test ./...` 中运行。
   - 通过环境变量开启，例如 `DPR_LIVE_TESTS=1`。
   - 只验证链上调用能跑通、metadata 数量非异常、已知账号能读出非空仓位。
   - 真实地址样本固定在 `protocols/aavev3/testdata/live_accounts.json`，测试会记录实际读到的 position 类型和 token symbol，但不做金额正确性比对。

3. Nightly / full regression tests
   - 使用 DeBank Aave V3 pools / holders 页面挑选并定期刷新测试账号。
   - 对 DeBank 结果只做人工或脚本辅助选样，不把 DeBank 实时 API 作为单元测试依赖。

## 测试账号数量建议

当前 `aave-v3` adapter 覆盖 Ethereum、Arbitrum One、Base，并支持 `lending` 和 `yield` 两类策略。

| 策略 | 每条链正例账号 | 每条链反例账号 | 选择标准 |
| --- | ---: | ---: | --- |
| lending | 3 | 1 | supply-only、supply+borrow、多资产/多市场 |
| yield | 2 | 1 | 一个大 TVL vault、一个小 TVL 或低用户数 vault |

完整回归建议：

- `lending`: 3 chains * 3 positive = 9 positive accounts。
- `yield`: 3 chains * 2 positive = 6 positive accounts。
- empty/no-position: 3 chains * 1 = 3 accounts。
- 总量约 18 条测试用例；账号可以跨策略复用，实际唯一地址通常会少一些。

PR smoke 建议收敛到：

- 每条链 `lending` 1 个正例。
- 每条链 `yield` 1 个正例。
- 全局 1 个 no-position 反例。

这样能把 public RPC 成本控制住，同时保留主要链路覆盖。

## 当前固定样本

当前样本来自 DeBank Aave V3 pools / holders 页面，记录了 2026-05-23 观察到的真实地址。它们的主要用途是沉淀测试数据和手工复现入口，不把 DeBank 的实时余额作为自动断言来源。

| ID | Chain | Strategy | Address | 预期观察 |
| --- | --- | --- | --- | --- |
| `ethereum-core-lending-supply-borrow` | Ethereum | lending | `0x28a55c4b4f9615fde3cdaddf6cc01fcf2e38a6b0` | Core market，WETH / wstETH supply，USDC / USDT debt |
| `ethereum-core-yield-waethusdt` | Ethereum | yield | `0xa484ab92fe32b143aee7019fc1502b1daa522d31` | StataToken yield，`waEthUSDT` -> `USDT` |
| `ethereum-gho-savings-gap` | Ethereum | unsupported-yield | `0xb0f8c20b849886cad270d705acc39739c0683f64` | DeBank Yield 中的 GHO/Savings 类样本，当前 adapter 先作为覆盖缺口保留 |
| `base-core-lending-supply-borrow` | Base | lending | `0xb3277d631f2cb651b3cbc5a54fccdabe69d12942` | Core market，WETH supply，USDC / WETH / cbBTC debt |
| `base-core-yield-wabasusdc` | Base | yield | `0xba1333333333a1ba1108e8412f11850a5c319ba9` | 多个 Base StataToken vault，例如 `waBasUSDC` |
| `arbitrum-core-lending-usdc` | Arbitrum One | lending | `0xa5b0edf6b55128e0ddae8e51ac538c3188401d41` | Core market，USDC supply |

## 运行方式

默认测试只校验 fixture 结构：

```bash
GOTOOLCHAIN=local go test ./protocols/aavev3
```

真实 RPC smoke test 需要显式开启：

```bash
DPR_LIVE_TESTS=1 GOTOOLCHAIN=local go test ./protocols/aavev3 -run TestLiveAaveV3PositionsFromFixtures -v
```

可以用 `DPR_LIVE_TEST_FILTER` 缩小范围，filter 会匹配 fixture id、chain 或 strategy：

```bash
DPR_LIVE_TESTS=1 DPR_LIVE_TEST_FILTER=base GOTOOLCHAIN=local go test ./protocols/aavev3 -run TestLiveAaveV3PositionsFromFixtures -v
```

## DeBank 选样方式

- 从 `https://debank.com/protocols/aave3/pools` 获取 pool 分类、链、TVL、用户数。
- 从 `https://debank.com/protocols/aave3/holders` 选 top holders。
- 对候选地址打开 portfolio 页面，确认它在 `aave3` / `base_aave3` / 对应链项目下有目标 strategy。
- 选定后将地址固定到 live test fixture，避免测试运行时依赖 DeBank 可用性。
