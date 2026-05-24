# DPR CLI 设计

本文记录 `cmd/dpr` 当前 CLI 能力和展示约定。它不是未来方案草稿，而是当前实现的使用说明和后续演进边界。

CLI 主要服务两类场景：

1. 快速查看某个地址在协议里的资产仓位。
2. 排查协议 adapter 在 metadata、cache、RPC 和 Multicall 阶段到底做了什么。

## 快速判断

普通用户优先使用 table：

```bash
dpr positions -chain base -protocol aave-v3 -address 0x... -format table
```

接入调试优先使用 detail + trace：

```bash
dpr sync-metadata -chain base -protocol aave-v3 -format detail -trace
dpr explain -chain base -protocol aave-v3 -format detail
dpr positions -chain base -protocol aave-v3 -address 0x... -format detail -trace -trace-level calls
```

程序消费继续使用 JSON，JSON 也是当前默认输出：

```bash
dpr positions -chain base -protocol aave-v3 -address 0x... -format json
```

## 命令概览

| Command | 作用 | 输出能力 |
| --- | --- | --- |
| `dpr chains` | 列出内置链配置，包括 chain id、RPC env 和 public RPC fallback | JSON |
| `dpr protocols [-chain]` | 列出已注册协议，或按链过滤 | JSON |
| `dpr sync-metadata -chain <chain> -protocol <id>` | 运行协议公共 metadata 同步并写入 SQLite cache | `json` / `table` / `detail` / `trace` |
| `dpr explain -chain <chain> -protocol <id>` | 查看协议配置和 metadata cache 状态，不读取用户资产 | `json` / `table` / `detail` / `trace` |
| `dpr positions -chain <chain> -protocol <id> -address <owner>` | 读取用户协议仓位；会先检查 metadata 是否存在且未过期 | `json` / `table` / `detail` / `trace` |

`sync-metadata`、`explain`、`positions` 支持：

```text
-format json|table|detail
-trace
-trace-level summary|calls|raw
```

`raw` 目前是保留级别：CLI 接受该参数，但不会打印 calldata 或 return data。当前实际可用层级是 `summary` 和 `calls`。

## 推荐工作流

### 1. 查看支持范围

```bash
dpr chains
dpr protocols -chain base
```

这一步用于确认当前内置哪些链、哪些协议，以及协议是否支持目标链。

### 2. 同步公共数据

```bash
dpr sync-metadata -chain base -protocol aave-v3 -format detail -trace
```

对 Aave V3 来说，这一步会写入三类 metadata：

| Namespace | 作用 |
| --- | --- |
| `markets` | 当前链上支持的 Aave V3 Market 配置 |
| `lending-reserves` | Reserve、underlying、aToken、debt token 和风险配置 |
| `yield-vaults` | StataToken / static aToken wrapper 信息 |

detail 输出会展示 discovery 阶段；trace 输出会展示这些链上读取通过 Multicall3 批量完成。

### 3. 检查 cache 状态

```bash
dpr explain -chain base -protocol aave-v3 -format detail
```

`explain` 不读取用户资产，只检查协议描述和 metadata cache。它适合在 `positions` 报 metadata missing / stale 时使用。

### 4. 查询用户资产

```bash
dpr positions -chain base -protocol aave-v3 -address 0x... -format table -trace
```

table 输出聚焦资产展示：

| 字段 | 含义 |
| --- | --- |
| `TYPE` | Position 类型，例如 `lending` 或 `vault` |
| `SUPPLY/SHARES` | Lending 的供应资产，或 Vault 的份额凭证 |
| `UNDERLYING` | Vault share 穿透后的底层资产；Lending 中通常省略为 `-` |
| `DEBT` | 借贷类协议中的当前债务 |
| `HEALTH` | Aave V3 Lending 的 health factor |

### 5. 排查底层链路

```bash
dpr positions -chain base -protocol aave-v3 -address 0x... -format detail -trace -trace-level calls
```

detail 输出会展开每个 Position 的 `Extra`，trace 会展示 cache 读取、metadata freshness、Aave V3 fetch 摘要和 Multicall3 批量调用摘要。

## 输出模式

### JSON

默认模式，适合脚本和测试。`positions` 默认输出 `[]Position`；开启 `-trace` 后会输出包装对象，包含 `chain`、`owner`、`positions`、`metadata` 和 `trace`。

### Table

面向快速查看。`positions -format table` 会把统一 Position 压缩成一行一仓位，适合确认用户是否有资产、供应资产和债务是什么。

### Detail

面向排查。`sync-metadata -format detail` 会展示 discovery；`positions -format detail` 会展示完整 Position 和 metadata cache；`explain -format detail` 会展示协议描述和 cache 状态。

### Trace

Trace 是观测信息，不参与仓位计算，开启 trace 不应该改变业务结果。

| Level | 当前行为 |
| --- | --- |
| `summary` | 展示 CLI 请求、metadata cache 状态、service 汇总和 Aave V3 阶段摘要 |
| `calls` | 展开 sync discovery step 和 Aave V3 lending / yield fetch 摘要 |
| `raw` | 保留级别，当前不输出 calldata / return data |

当前 trace 主要由 CLI 根据 metadata snapshot、`SyncResult.Details` 和 `Position.Extra` 生成。后续如果需要跨协议统一的逐调用 trace，可以把 collector 下沉到 service、adapter 和 `pkg/evm`。

## Aave V3 展示映射

### Lending

`fetchMarketPosition` 会为每个有资产的 market 输出一个 lending position：

| Position 字段 | Aave V3 来源 |
| --- | --- |
| `Shares` | 当前 aToken balance |
| `Underlying` | 与 aToken 对应的底层供应资产 |
| `Debt` | `currentStableDebt + currentVariableDebt` |
| `Extra.healthFactorFormatted` | `Pool.getUserAccountData` |
| `Extra.reserves` | 每个活跃 reserve 的 supply、stable debt、variable debt、collateral flag 等 |

### Yield

`fetchYieldPositions` 会为每个有余额的 vault 输出一个 vault position：

| Position 字段 | Aave V3 来源 |
| --- | --- |
| `Shares` | `balanceOf(owner)` 得到的 vault share |
| `Underlying` | `previewRedeem(shares)` 得到的可赎回底层资产 |
| `Extra.vaultKind` | `stata-token` 或 `legacy-static-a-token` |
| `Extra.maxWithdrawFormatted` | `maxWithdraw(owner)` |
| `Extra.claimableRewards` | vault reward token 查询结果 |

## 当前边界

- CLI 不做价格估值，也不展示用户 PnL。
- Aave V3 页面上的 APR / borrow APR 不是当前 CLI 展示重点；现阶段重点是仓位、底层资产、债务和 health factor。
- `raw` trace 级别暂不打印 calldata / return data，避免默认暴露过多低层数据。
- public RPC fallback 只适合演示或低频调试；真实使用建议配置自有 RPC URL。
