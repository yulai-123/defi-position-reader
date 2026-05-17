# DeFi Position Reader

DeFi Position Reader 是一个用 Go 编写的轻量 DeFi 协议资产读取与解析框架。

它关注的不是普通钱包余额，而是用户在 DeFi 协议里的仓位：LP 份额、Vault Share、借贷凭证、债务、质押记录、待赎回资产等。项目目标是用一套清晰、可运行、可测试的框架，逐步展示不同协议的链上仓位如何被读取，并尽量穿透到底层 Token。

> 当前项目处于框架阶段，内置 `demo` 协议用于离线演示 metadata sync、cache 和 position fetch 闭环；真实协议会按 adapter 逐步接入。

## 项目定位

- 面向 EVM 多链，默认包含 Ethereum、Arbitrum One、Base。
- 每个协议通过独立 Adapter 接入，协议知识收敛在 `protocols/{protocol-id}` 目录。
- Adapter 内部拆分 `MetadataSyncer` 和 `PositionFetcher`，分别负责协议公共数据同步和用户仓位读取。
- 输出统一收敛到 `Position` 模型，包含 `shares`、`underlying`、`debt` 和协议扩展字段。
- 不做完整商业索引器，不承诺强一致区块快照，不包含价格估值和 HTTP API。
- 测试优先使用 mock / fixture 数据，保证没有 RPC key 也能执行 `go test ./...`。

## 架构概览

![项目架构数据流](./docs/diagrams/architecture-flow.png)

核心流程可以概括为：

```text
CLI
  -> Protocol Registry
    -> Protocol Adapter
      -> MetadataSyncer -> Metadata Store
      -> PositionFetcher -> []Position
```

`MetadataSyncer` 维护协议公共数据，例如 market、reserve、pool、token、oracle config；`PositionFetcher` 读取单个用户地址的协议仓位，并结合 metadata 解析成统一的 `Position`。

更详细的架构说明见 [docs/architecture.md](./docs/architecture.md)，框架设计博客见 [docs/blogs/000-defi-asset-position-framework.md](./docs/blogs/000-defi-asset-position-framework.md)。

## 目录结构

```text
cmd/dpr/          CLI 入口
pkg/core/         通用模型：Chain、Token、TokenAmount、Position、MetadataInfo
pkg/adapter/      协议接入接口和 Protocol Registry
pkg/cache/        metadata cache 接口和 SQLite 实现
pkg/chain/        默认 EVM 链配置
pkg/service/      薄编排层，串联 registry、adapter 和 metadata store
protocols/demo/   离线演示协议，展示 metadata sync + position fetch 闭环
docs/architecture.md  架构速查
docs/blogs/       技术博客
docs/diagrams/    文档配图
```

## 快速开始

```bash
go test ./...
make vet
make race
make cover

go run ./cmd/dpr chains
go run ./cmd/dpr protocols
go run ./cmd/dpr sync-metadata -chain ethereum -protocol demo
go run ./cmd/dpr positions -chain ethereum -protocol demo -address 0x0000000000000000000000000000000000000001
```

也可以直接运行 demo 闭环：

```bash
make run-demo
```

当前 `demo` 协议使用内置 fixture 数据，不需要 RPC key。后续接入真实协议时，再使用 `.env.example` 中的 RPC 配置。

## 示例输出

`positions` 命令会输出统一的 Position 结果。下面是 demo 协议中的 LP 仓位精简示例，展示了 share token 到 underlying token 的解析关系；完整输出还会包含 position id、owner、token address、decimals 和 raw amount。

```json
{
  "protocol": "demo",
  "type": "liquidity",
  "displayName": "Demo WETH / USDC LP",
  "shares": [
    { "token": { "symbol": "dLP" }, "formatted": "1.5" }
  ],
  "underlying": [
    { "token": { "symbol": "WETH" }, "formatted": "0.42" },
    { "token": { "symbol": "USDC" }, "formatted": "1234.5" }
  ]
}
```

## 当前支持

| 类型 | 状态 |
| --- | --- |
| Chains | Ethereum, Arbitrum One, Base |
| Protocols | `demo` fixture adapter |
| Cache | SQLite metadata store |
| CLI | `chains`, `protocols`, `sync-metadata`, `positions` |
| Tests | `go test ./...`, `go test ./... -race`, coverage 90%+ |

## 协议接入路线

后续会按协议逐个接入，并为每个协议补充资产解析文章。候选协议包括：

- Lido：staking / wrapper 类资产，适合展示 stETH、wstETH 与底层 ETH 的关系。
- Aave V3：借贷协议，适合展示 reserve metadata、aToken、debt token、underlying 和 debt。
- Uniswap V2：经典 LP Token，适合展示池子储备和份额换算。
- Uniswap V3：NFT LP 仓位，适合展示 tick、liquidity 和底层资产估算。
- Compound V3：单一基础资产借贷模型，适合和 Aave V3 对比。
- Sky：MakerDAO / Sky 生态资产，适合展示稳定币、储蓄和协议特定状态。

建议接入顺序是：Lido -> Aave V3 -> Uniswap V2 -> Uniswap V3 -> Compound V3 -> Sky。这样可以从简单凭证型资产，逐步过渡到借贷、LP、NFT LP 和更复杂的协议状态。

## 技术博客

- [000 - DeFi 资产读取与解析：框架介绍](./docs/blogs/000-defi-asset-position-framework.md)

后续每接入一个真实协议，都会补充对应的协议资产解析文章。

## License

Apache-2.0
