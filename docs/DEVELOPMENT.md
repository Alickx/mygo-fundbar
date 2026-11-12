# 开发指南

[返回 README](../README.md)

FundBar 使用 Go 与 [MyGo](https://github.com/egoist/mygo) 构建 macOS 原生界面。Go 版本与依赖以 [`go.mod`](../go.mod) 为准，应用版本与打包配置见 [`mygo.json`](../mygo.json)。

## 环境与运行

需要 macOS 和 Go 1.27.3 或更新版本。MyGo CLI 已通过 Go tool 依赖声明，无需单独全局安装。

```sh
git clone https://github.com/Alickx/mygo-fundbar.git
cd mygo-fundbar
go run .
```

系统通知只在打包后的应用中可用：`go run .` 运行时，设置里的「提醒」会注明不支持；要试提醒，用 `go tool mygo dev` 运行。

## 质量检查

提交代码前运行与 CI 一致的检查。`gofmt -l .` 应无输出，其余命令应正常通过。

```sh
gofmt -l .
go vet ./...
go tool mygo vet .
go test -timeout 180s ./...
```

测试覆盖估值算法、接口解析和界面构建等逻辑。涉及界面或交互的改动，还应运行应用，检查浅色 / 深色模式与最小窗口下的实际表现。

## 构建

```sh
go tool mygo build -platform darwin/universal
open build/darwin-universal/FundBar.app
```

输出位于 `build/darwin-universal/`，包括通用应用 `FundBar.app` 和 `fundbar-<version>.dmg`，支持 Apple Silicon 与 Intel Mac。

带上更新签名私钥时，还会写出安装应用可直接更新的文件：`fundbar-<version>-darwin-universal.tar.gz`（安装后的应用）、`update-darwin-universal.json`（版本、说明、下载地址与签名）以及从最近几个版本升级的 `*.delta` 增量。

```sh
MYGO_UPDATER_PRIVATE_KEY="$(cat ~/Library/Application\ Support/mygo/update-keys/mygo-update.key)" \
  go tool mygo build -platform darwin/universal
```

没有私钥时只警告、不生成这些文件（本地开发无需配置）。

保留开发特性的构建：

```sh
go tool mygo build -debug
```

## 开发工具

### 对比估值与公布净值

```sh
go run ./tools/probe 161725 005827
```

该工具会访问数据接口，输出指定基金的估算结果与最新公布净值。比较时应核对行情日期与净值日期，单日样本不能代表长期精度。

### 界面截图

`tools/shot` 从本地缓存加载数据，通过离屏渲染生成 PNG，无需打开应用窗口。使用临时数据目录可预览示例持仓。

```sh
go run ./tools/shot /tmp/fundbar-light.png
go run ./tools/shot /tmp/fundbar-dark.png dark
go run ./tools/shot /tmp/fundbar-settings.png light settings
go run ./tools/shot /tmp/fundbar-edit.png light edit
go run ./tools/shot /tmp/fundbar-search.png light search
go run ./tools/shot /tmp/fundbar-empty.png light empty
FUNDBAR_GROUP=关注 go run ./tools/shot /tmp/fundbar-group.png light group
go run ./tools/shot /tmp/fundbar-panel.png light panel
go run ./tools/shot /tmp/fundbar-portfolio.png light portfolio
go run ./tools/shot /tmp/fundbar-ledger.png light ledger 007760
go run ./tools/shot /tmp/fundbar-ledger-add.png light ledger-add 007760
go run ./tools/shot /tmp/fundbar-ledger-edit.png light ledger-edit 007760
go run ./tools/shot /tmp/fundbar-ledger-error.png light ledger-error 007760
FUNDBAR_PERIOD=6 go run ./tools/shot /tmp/fundbar-risk.png light risk 007760
FUNDBAR_PERIOD=3 go run ./tools/shot /tmp/fundbar-recovery.png light recovery 007760
FUNDBAR_PERIOD=3 FUNDBAR_COMPARE=021528,159992,028678 \
  go run ./tools/shot /tmp/fundbar-compare.png light compare 007760
FUNDBAR_SHARE_MONEY=1 go run ./tools/shot /tmp/fundbar-share.png light share
```

参数顺序为：输出路径、主题（`light` / `dark`）、场景（`normal` / `settings` / `edit` / `search` / `batch` / `empty` / `group` / `groupnew` / `panel` / `portfolio` / `ledger` / `ledger-add` / `ledger-edit` / `ledger-error` / `risk` / `recovery` / `compare` / `share`）、选中的基金代码。`batch` 场景用 `FUNDBAR_BATCH` 指定搜索框里的代码串，展示批量添加行；设置窗口截图可用 `FUNDBAR_SCROLL` 指定滚动距离（如 `600`），把「数据」一节带进画面。`risk` 场景用 `FUNDBAR_PERIOD` 选择走势图的周期，序号依次为 0–6（近1月、近3月、近6月、近1年、今年来、近3年、成立以来），用来核对风险指标行与长周期的年份刻度。`recovery` 场景同样用 `FUNDBAR_PERIOD` 选周期，展示净值修复视图：回撤曲线、修复指标与图上的标注。`compare` 场景同样用 `FUNDBAR_PERIOD` 选周期，用 `FUNDBAR_COMPARE` 给出最多三只基金的代码，核对对比曲线与图例。`share` 场景不画窗口，而是直接写出分享图；`FUNDBAR_SHARE_MONEY=1` 展示含金额的版本。`group` 场景用 `FUNDBAR_GROUP` 指定要展示的分组名称。`ledger` 系列场景用第四个参数指定基金代码，并在内存中准备示例（或该基金已有的）交易记录，不会写入数据目录；`ledger-add` 和 `ledger-edit` 打开新增或编辑表单，`ledger-error` 展示交易记录有误时的提示行。`panel` 场景按系统毛玻璃渲染小窗，再铺一层灰色渐变作底，用来核对小窗在材质上的对比度。

```sh
FUNDBAR_DIR=/tmp/fundbar-preview FUNDBAR_W=900 FUNDBAR_H=600 \
  go run ./tools/shot /tmp/fundbar-compact.png light normal 007760
```

`FUNDBAR_DIR` 指定截图工具读取的数据目录；`FUNDBAR_W`、`FUNDBAR_H` 设置主窗口截图尺寸。设置窗口截图使用固定尺寸。

### 生成图标

```sh
go run ./tools/genicon
```

重新生成 `resources/` 下的菜单栏图标和应用图标。

## 代码结构

```text
main.go              应用入口、主窗口与菜单栏初始化
internal/
  app/               窗口、界面、刷新调度与状态管理
  calendar/          A 股休市日表与交易日判断
  ledger/            交易记录与持仓计算
  model/             数据类型与估值算法
  source/            基金资料与股票行情适配器
  store/             自选配置与本地缓存
resources/           应用图标与菜单栏图标
tools/
  probe/             估值对比工具
  shot/              离屏截图工具
  genicon/           图标生成工具
mygo.json            应用元数据与打包配置
```

## 每年更新交易日历

[`internal/calendar/calendar.go`](../internal/calendar/calendar.go) 的 `closures` 表列出周末之外的 A 股休市日，刷新调度与净值日期判断都读它。表未覆盖的年份按周一至周五视为交易日，因此每年发布新版时，应依据国务院办公厅的次年节假日安排（以及沪深交易所的休市安排）补上下一年：只列入落在周一至周五的休市日，周末调休上班日仍按周末休市。

2026 年的表由三只长历史基金都没有公布净值的工作日反推，并与公开的节假日安排核对；2027 年的表依据国务院办公厅《关于2027年部分节假日安排的通知》（国办发明电〔2026〕10号，2026-11-05）编制。

## 本地数据

默认目录为 `~/Library/Application Support/FundBar/`：

| 文件 | 用途 |
| --- | --- |
| `funds.json` | 自选基金、分组、份额、成本价、止盈止损、提醒开关、选中项及部分界面设置 |
| `ledger/<基金代码>.json` | 该基金的全部交易记录；份额与成本由它重放得出，`funds.json` 里保留一份镜像供旧版本读取 |
| `alerts.json` | 已发出的提醒记录：当天提醒过哪些基金，以及各基金是否已在止盈 / 止损区间内；重启后不重复发送 |
| `estimates.json` | 盘中估值与当晚公布净值的对照记录：每只基金最近 60 个交易日的估值、覆盖率与已公布涨跌，用于显示「估值偏差」 |
| `cache/<基金代码>.json` | 基金资料、历史净值、持仓与行情快照 |
| `backups/导入前 <时间>.json` | 每次导入备份前自动留下的旧数据，文件名如 `导入前 20261112-041500.json` |

可在应用设置中打开数据目录或清空缓存。备份自选与手动持仓时，请保留 `funds.json`；使用了交易记录的基金还要保留 `ledger/` 目录。`alerts.json` 与 `estimates.json` 只是本地记录，不需要备份。

### 备份与迁移

设置 → 数据里的「导出备份…」把 `funds.json`（自选、分组、份额、成本价与界面设置）和 `ledger/` 里的交易记录写成一个 JSON 文件：`{"app":"FundBar","version":1,"exported":"<RFC 3339>","funds":{...},"ledgers":{"<基金代码>":[...]}}`。缓存、`alerts.json` 与 `estimates.json` 不在其中：前者是网络能再取回的资料，后两者是本机的提醒与估值记录。换电脑时把这个文件带走即可。

「导入备份…」会先校验应用名、版本、自选数据与每一笔交易记录，任何一项不合格都会说明原因并保持原样；确认导入后，当前数据会先写进 `backups/导入前 <时间>.json`，再替换自选与交易记录，最后重新载入界面。持仓表可用「导出 CSV…」导出为带 BOM 的 UTF-8 表格，Excel 能直接打开。

## 签名与发布

默认构建使用 ad-hoc 签名。若使用 Developer ID 签名，先查看可用身份：

```sh
security find-identity -v -p codesigning

go tool mygo build -platform darwin/universal \
  -sign "Developer ID Application: Your Name (TEAMID)"
```

Apple 公证需要单独配置。使用 `xcrun notarytool store-credentials` 保存凭据后，在 `mygo.json` 的 `macos` 下配置 `notarize.keychainProfile`，例如 `AC_PASSWORD`。签名校验通过本身不代表已完成公证。

[`build.yml`](../.github/workflows/build.yml) 在推送到 `main`、推送 `v*` 标签、提交 Pull Request 或手动触发时运行：

1. 检查格式、静态分析与测试。
2. 构建 macOS 通用应用与 DMG，并签名更新包与更新清单。
3. 使用 `codesign --verify --deep --strict` 校验签名。
4. 上传应用与安装包作为构建产物。
5. 对 `v*` 标签创建 Release，附带 DMG、更新归档、更新清单与增量包。

配置仓库 Secrets `MACOS_SIGNING_IDENTITY`、`MACOS_CERT_P12`（base64）和 `MACOS_CERT_PASSWORD` 后，CI 会导入证书并签名；未配置时使用 ad-hoc 签名。当前工作流没有配置公证凭据的执行步骤，发布公证版本时还需补齐该流程。

## 自动更新

应用从 GitHub Release 更新：既不占用服务器，也不需要域名。安装后的应用读取
`https://github.com/Alickx/mygo-fundbar/releases/latest/download/update-darwin-universal.json`，版本更高时下载增量或完整归档，用编译进应用的公钥校验 Ed25519 签名，重启后即为新版本。这一功能由 MyGo 的更新器提供，细节见其 [auto-updates](https://github.com/egoist/mygo/blob/main/docs/updates.md) 文档。

界面在应用自己的窗口里，不弹单独窗口：`internal/app/update.go` 负责检查、下载与状态机，`updateStrip`（`internal/app/view.go`）画在摘要栏下方，状态依次为「发现有新版本 → 下载更新（带进度条）→ 安装并重启」，另有「忽略」（记在 `funds.json` 的 `skippedVersion`，下一个版本照常提示）与「更新说明」（展开 CHANGELOG 原文）。应用菜单与菜单栏右键菜单的「检查更新…」走同一条路径，结果写在窗口底部状态行几秒。

因此本应用**不使用** `plugins/updater` 或 `plugins/updater/native`：那两个插件自带一个独立更新窗口。改界面时可查看 `go run ./tools/shot /tmp/u.png light update`（`FUNDBAR_STEP=fetching|ready`、`FUNDBAR_NOTES`、`FUNDBAR_NOTES_OPEN=1`）。

更新签名密钥对只需生成一次：

```sh
go tool mygo keygen        # 私钥与公钥写到 ~/Library/Application Support/mygo/update-keys/
```

- **公钥**：写入 `mygo.json` 的 `updates.publicKey`，随代码提交。
- **私钥**：绝不提交；本机与离机各留一份备份，并设为仓库 Secret `MYGO_UPDATER_PRIVATE_KEY`（`gh secret set`）。只依赖 GitHub Secret 不够，因为 Secret 无法读回。

私钥丢失意味着已安装的应用再也收不到更新；私钥泄露意味着他人可以向所有用户推送任意代码，应立即改用新密钥对重新发布。清单里只有一把公钥，`mygo build` 也不会用与 `publicKey` 不匹配的私钥签名，因此无法在用户不停机的情况下换钥：换钥意味着旧版本用户收不到后续更新，需要手动下载一次。

发布新版本的步骤：

1. 递增 `mygo.json` 的 `version`，并同步 `internal/app/settings.go` 里的 `previewVersion`（截图预览用）。
2. 在 [`CHANGELOG.md`](../CHANGELOG.md) 顶部新增 `## <version>` 小节，内容即更新窗口展示给用户的说明；缺少该小节时构建会失败。
3. 提交后打标签 `v<version>` 并推送，CI 构建、签名并发布带更新文件的 Release。

标签号与 `mygo.json` 的 `version` 必须一致：清单里的下载地址按 `v<version>` 生成。只带 updater 的版本才能自动更新，因此首个附带更新器的版本（0.4.0）是自动更新的起点，此前的版本需手动下载安装一次。
