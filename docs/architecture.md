# 架构说明

这份文档是项目的代码架构速查，重点回答三个问题：

1. 命令进来后，代码如何流动。
2. 每个模块负责什么。
3. 接入一个真实 DeFi 协议时，需要实现哪些边界。

如果想了解为什么要这样拆分，请先看 [框架介绍博客](./blogs/000-defi-asset-position-framework.md)。

## 核心数据流

项目围绕 CLI、Protocol Registry、Protocol Adapter、Metadata Store 和 Position 模型组织代码。

```text
CLI
  -> service
    -> Protocol Registry
      -> Protocol Adapter
        -> MetadataSyncer -> Metadata Store
        -> PositionFetcher -> []Position
```

![项目架构数据流](./diagrams/architecture-flow.png)

`service` 只是较薄的编排层，核心扩展点仍然是 Protocol Adapter。每个协议 Adapter 内部封装自己的公共数据同步逻辑和用户仓位读取逻辑，框架层只负责注册、选择、调用和收敛输出。

## 执行链路

### sync-metadata

`sync-metadata` 用来同步协议公共数据，例如 market、reserve、pool、token、vault / farming config。

```text
cmd/dpr
  -> service.SyncMetadata
    -> registry.Resolve(protocols, chain)
      -> adapter.MetadataSyncer.Sync
        -> cache.Store.Set
```

当前设计中，框架会把 `cache.Store` 传给 Syncer；是否写入缓存、写入哪些 namespace，由具体协议 Syncer 决定。

### positions

`positions` 用来读取某个用户地址在指定协议里的仓位，并输出统一的 `Position`。

```text
cmd/dpr
  -> service.FetchPositions
    -> registry.Resolve(protocols, chain)
      -> adapter.PositionFetcher.Fetch
        -> cache.Store.Get
        -> []core.Position
```

Fetcher 可以读取 Metadata Store 中已有的协议公共数据，再结合用户维度的链上状态生成 `Position`。当前 `demo` 协议为了离线演示，在没有缓存时会使用内置 fixture 数据；真实协议可以选择更严格的策略，例如 `aave-v3` 会要求 `markets`、`lending-reserves` 和 `yield-vaults` metadata 都存在且未过期，`compound-v3` 会要求 `markets`、`collateral-assets` 和 `reward-configs` metadata 都存在且未过期，`uniswap-v2` 会要求 `markets`、`pairs` 和 `farming-pools` metadata 都存在且未过期，否则直接提示先运行 `sync-metadata`，避免静默漏资产。

## 模块职责

| 模块 | 职责 |
| --- | --- |
| `cmd/dpr` | CLI 入口，解析命令参数并输出 `json`、`table` 或 `detail`，并可附带 trace。 |
| `pkg/chain` | 默认 EVM 链配置，包括 chain id、名称、RPC 环境变量名和 public RPC fallback。 |
| `pkg/core` | 跨模块共享的数据模型，例如 `Chain`、`Token`、`TokenAmount`、`Position`、`MetadataInfo`。 |
| `pkg/adapter` | 协议接入接口、`MetadataSyncer`、`PositionFetcher` 和 Protocol Registry。 |
| `pkg/cache` | metadata cache 抽象和 SQLite 实现。 |
| `pkg/evm` | EVM RPC client、Multicall3 调用封装和数值格式化工具。 |
| `pkg/service` | 薄编排层，串联 Registry、Adapter 和 Metadata Store。 |
| `protocols/demo` | 离线演示协议，用 fixture 跑通 metadata sync 和 position fetch 闭环。 |
| `protocols/aavev3` | Aave V3 真实协议接入，支持 Lending 和 StataToken / static aToken Yield 仓位。 |
| `protocols/compoundv3` | Compound V3 / Comet 真实协议接入，支持 base asset Yield、collateralized Lending 和 Reward 仓位。 |
| `protocols/uniswapv2` | Uniswap V2 真实协议接入，支持直接 LP、Farming 和 Reward 仓位。 |
| `docs/blogs` | 技术博客，用来记录框架设计和后续协议接入过程。 |

## Adapter 接入规范

接入一个新协议时，建议在 `protocols/{protocol-id}` 下维护协议自己的 Adapter。

一个 Adapter 需要提供三部分能力：

```go
type Adapter interface {
	Descriptor() core.ProtocolDescriptor
	Syncer() MetadataSyncer
	Fetcher() PositionFetcher
}
```

### Descriptor

`Descriptor` 描述协议基础信息：

- 协议 ID，例如 `aave-v3`、`lido`、`uniswap-v2`。
- 协议名称和分类。
- 支持的 chain id 列表。

Protocol Registry 会根据 `chain` 和 `protocol` 筛选需要执行的 Adapter。

### MetadataSyncer

`MetadataSyncer` 负责同步协议公共数据。公共数据通常不依赖用户地址，但会被 Fetcher 用来解析仓位。

常见 metadata 示例：

- Aave V3：market 列表、reserve 列表、aToken、variable debt token、stable debt token、StataToken / static aToken vault。
- Compound V3：Comet market 列表、base asset、collateral asset、price feed、collateral factor、reward config。
- Uniswap V2：factory、pair 列表、pair token0/token1、farming pool 配置。
- Uniswap V3：position manager、factory、pool 配置、tick spacing。
- Lido：stETH、wstETH、兑换关系和部署地址。

缓存 key 按 `chainId / protocol / namespace` 组织。真实协议可以用 namespace 拆分不同数据集，例如 Aave V3 当前使用 `markets`、`lending-reserves` 和 `yield-vaults`，Compound V3 当前使用 `markets`、`collateral-assets` 和 `reward-configs`，Uniswap V2 当前使用 `markets`、`pairs` 和 `farming-pools`。

### PositionFetcher

`PositionFetcher` 负责读取单个用户地址的协议资产，并输出统一的 `[]core.Position`。

Fetcher 通常会做三件事：

1. 读取用户仓位，例如 ERC20 凭证余额、NFT position、借贷余额、内部 mapping 状态。
2. 读取或复用协议公共 metadata。
3. 把协议原始状态映射成 `Position`，尽量填充 `shares`、`underlying` 和 `debt`。

## 数据模型

### Position

`Position` 是面向用户的资产结果：

```go
type Position struct {
	ID          string
	ChainID     int64
	Protocol    string
	Owner       string
	Type        PositionType
	DisplayName string
	Shares      []TokenAmount
	Underlying  []TokenAmount
	Debt        []TokenAmount
	Extra       map[string]any
}
```

字段含义：

- `Shares`：用户持有的份额或凭证，例如 LP Token、aToken、Vault Share、NFT 代表的流动性份额。
- `Underlying`：该仓位穿透后对应的底层资产。
- `Debt`：借贷类协议中的债务资产。
- `Extra`：协议特定扩展信息，适合放 market id、pool id、token id、health factor 等非通用字段。

### MetadataInfo

`MetadataInfo` 描述一份缓存 metadata 的版本和来源：

```go
type MetadataInfo struct {
	ChainID     int64
	Protocol    string
	Namespace   string
	Version     string
	BlockNumber uint64
	UpdatedAt   time.Time
	Source      string
}
```

当前 `Position` 不承诺所有数据来自同一个区块快照。`MetadataInfo.BlockNumber` 会保留 metadata 同步时的区块信息，后续可以用于数据新鲜度展示或解析 trace。

## 当前边界

当前项目仍然保持轻量，暂不包含：

- 钱包原生资产、普通 ERC20、NFT 持仓扫描。
- 价格系统和资产估值。
- HTTP API 或长期运行的索引服务。
- 强一致区块快照。
- 原始 calldata / return data 级别的深度 trace。

CLI 已经支持 `-format json|table|detail` 和 `-trace`，用于展示 metadata cache、discovery 和部分 adapter 调用链路；后续如果多个协议都需要更细粒度 trace，可以再把 trace collector 下沉到 service / adapter / evm 层。框架层优先保持稳定、清晰和容易测试。
