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
```

参数顺序为：输出路径、主题（`light` / `dark`）、场景（`normal` / `settings` / `edit` / `search` / `empty` / `group` / `groupnew` / `panel`）、选中的基金代码。`group` 场景用 `FUNDBAR_GROUP` 指定要展示的分组名称。`panel` 场景按系统毛玻璃渲染小窗，再铺一层灰色渐变作底，用来核对小窗在材质上的对比度。

```sh
FUNDBAR_DIR=/tmp/fundbar-preview FUNDBAR_W=900 FUNDBAR_H=600 \
  go run ./tools/shot /tmp/fundbar-compact.png light normal 007760
```

`FUNDBAR_DIR` 指定截图工具读取的数据目录；`FUNDBAR_W`、`FUNDBAR_H` 设置主窗口截图尺寸。设置窗口截图使用固定尺寸。

### 更新 README 宣传图

```sh
go run ./tools/promo
```

直接用当前源码渲染浅色 / 深色主界面与菜单栏小窗，并以 2 倍分辨率合成海报，输出到 `docs/promo/`。全程本地执行；读取行情缓存，使用独立的示例持仓配置。修改 UI 后运行此命令即可同步宣传图。输入缓存、参数、截图口径与产物说明见 [宣传图生成指南](promo/README.md)。

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
  model/             数据类型与估值算法
  source/            基金资料与股票行情适配器
  store/             自选配置与本地缓存
resources/           应用图标与菜单栏图标
tools/
  probe/             估值对比工具
  shot/              离屏截图工具
  promo/             当前界面截图与 README 宣传图生成器
  genicon/           图标生成工具
mygo.json            应用元数据与打包配置
```

## 本地数据

默认目录为 `~/Library/Application Support/FundBar/`：

| 文件 | 用途 |
| --- | --- |
| `funds.json` | 自选基金、分组、份额、成本价、选中项及部分界面设置 |
| `cache/<基金代码>.json` | 基金资料、历史净值、持仓与行情快照 |

可在应用设置中打开数据目录或清空缓存。备份自选与手动持仓时，请保留 `funds.json`。

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
- **私钥**：绝不提交，至少两处备份（本机与离机各一份）。macOS 自带的钥匙串就能当保险箱，不必装额外软件：

```sh
KEY=~/Library/Application\ Support/mygo/update-keys/mygo-update.key
security add-generic-password -a fundbar -s "FundBar update signing key" -w "$(cat "$KEY")" -U

# 设置 CI 的 Secret，不必把私钥写到文件或终端历史里
security find-generic-password -a fundbar -s "FundBar update signing key" -w \
  | gh secret set MYGO_UPDATER_PRIVATE_KEY
```

取回（换机、重新生成 CI 凭据时）：

```sh
security find-generic-password -a fundbar -s "FundBar update signing key" -w
```

另一份建议抄在纸上、放在证件一类物品里，或存进 U 盘等离线介质；只依赖 GitHub Secret 不够，因为 Secret 无法读回。

私钥丢失意味着已安装的应用再也收不到更新；私钥泄露意味着他人可以向所有用户推送任意代码，应立即改用新密钥对重新发布。清单里只有一把公钥，`mygo build` 也不会用与 `publicKey` 不匹配的私钥签名，因此无法在用户不停机的情况下换钥：换钥意味着旧版本用户收不到后续更新，需要手动下载一次。

发布新版本的步骤：

1. 递增 `mygo.json` 的 `version`，并同步 `internal/app/settings.go` 里的 `previewVersion`（截图预览用）。
2. 在 [`CHANGELOG.md`](../CHANGELOG.md) 顶部新增 `## <version>` 小节，内容即更新窗口展示给用户的说明；缺少该小节时构建会失败。
3. 提交后打标签 `v<version>` 并推送，CI 构建、签名并发布带更新文件的 Release。

标签号与 `mygo.json` 的 `version` 必须一致：清单里的下载地址按 `v<version>` 生成。只带 updater 的版本才能自动更新，因此首个附带更新器的版本（0.4.0）是自动更新的起点，此前的版本需手动下载安装一次。
