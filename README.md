# FundBar 基金估值

一个住 macOS 菜单栏的基金估值小工具：实时估值、自选列表、历史净值曲线，状态栏直接显示今天赚了多少。

![light](docs/screenshot.png)
![dark](docs/screenshot-dark.png)

## 为什么自己算估值

2023 年之后各大平台的「实时估值」陆续下线，官方接口也没了：

| 接口 | 现状 |
|---|---|
| `fundgz.1234567.com.cn/js/{code}.js` | 404，已下线 |
| `fundmobapi.eastmoney.com/…/FundMNFInfo` | 200，但 `GSZ`/`GSZZL` 全是 `null` |
| 蛋卷 `/djapi/fund/estimate/{code}` | 需要登录 |
| 雪球 `quote.json` | 需要 token |

所以本项目的估值是**自己算的**：

```
估值涨跌% = Σ(重仓股占净值比例 × 该股票今日涨跌%) ÷ Σ(重仓股占净值比例)
```

也就是用最新季报的前十大重仓股，按占比归一化后加权。净值本身仍来自官方（当日净值每天晚上公布），估值只在净值公布前替代它。

### 实测精度（2026-11-09 收盘价 vs 当日公布净值）

| 基金 | 类型 | 重仓覆盖 | 估算 | 实际 | 误差 |
|---|---|---|---|---|---|
| 161725 招商中证白酒 | 指数 | 86.4% | -0.69% | -0.63% | -0.06pp |
| 110022 易方达消费行业 | 股票 | 72.5% | -0.65% | -0.63% | -0.02pp |
| 003096 中欧医疗健康C | 偏股混合 | 74.8% | -1.16% | -0.84% | -0.32pp |
| 005827 易方达蓝筹精选 | 偏股混合 | 42.6% | -0.43% | -0.21% | -0.22pp |

覆盖越低误差越大。覆盖不到 20% 的基金（如 ETF 联接、QDII）不显示估值，只显示官方净值。

## 功能

- **实时估值**：交易时段每 45 秒刷新，收盘后显示已公布净值
- **自选列表**：搜索代码/名称添加，右键移出，拖动排序
- **历史净值**：近 1 月 / 3 月 / 6 月 / 1 年，鼠标划过读数
- **今日收益**：填了份额和成本价后，状态栏显示当日收益，面板显示累计收益
- **状态栏**：左键开面板，右键出菜单（含每只基金的涨跌）

## 数据来源

| 用途 | 接口 |
|---|---|
| 基金名 / 全部历史净值 / 股票仓位 / 持仓代码 | `fund.eastmoney.com/pingzhongdata/{code}.js` |
| 前十大重仓股及占净值比例 | `fundf10.eastmoney.com/FundArchivesDatas.aspx?type=jjcc` |
| 个股实时价格（A 股 + 港股，一次请求批量） | `qt.gtimg.cn/q=sh600519,sz000858,hk00700` |
| 基金搜索 | `fundsuggest.eastmoney.com/FundSearch/api/FundSearchAPI.ashx` |

都是公开接口，不需要 key；请控制频率，别把人家打挂。

## 运行

需要 Go 1.27.3+：

```sh
go run .                        # 直接跑
go run github.com/egoist/mygo/cmd/mygo@v0.8.3 build   # 出 FundBar.app 和 dmg
```

打包产物在 `build/darwin-arm64/`：`FundBar.app`（11 MB）和 dmg（3.6 MB）。

自选基金存在 `~/Library/Application Support/FundBar/funds.json`，抓到的数据缓存在同目录 `cache/`，删掉即可重来。

## 开发

```sh
go test ./...                                    # 单元测试（含估值算法、接口解析）
go run ./tools/probe 161725 005827               # 对比估算值和公布净值
go run ./tools/shot /tmp/shot.png [dark]         # 不开窗口，把界面渲染成 PNG
go run github.com/egoist/mygo/cmd/mygo@v0.8.3 vet .   # 检查 native UI 的构建生命周期
```

代码结构：

```
main.go                    窗口 / 菜单栏图标 / 应用菜单
internal/model/            数据类型与估值算法（纯函数）
internal/source/           天天基金、腾讯行情的适配器
internal/store/            funds.json 与缓存
internal/app/app.go        状态、刷新调度、汇总
internal/app/view.go       界面
internal/app/chart.go      净值曲线
tools/probe                估值精度自查
tools/shot                 离屏渲染截图
```

界面用 MyGo 的原生 UI（Go 直接画，没有 webview、没有 JS），所以启动快、内存低。

## 免责

估值由季报重仓股推算，**不是官方数据**，仅供参考。实际净值以基金公司公布为准。本项目不构成任何投资建议。

## License

MIT
