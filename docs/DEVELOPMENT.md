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
```

参数顺序为：输出路径、主题（`light` / `dark`）、场景（`normal` / `settings` / `edit` / `search` / `empty`）、选中的基金代码。

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

## 本地数据

默认目录为 `~/Library/Application Support/FundBar/`：

| 文件 | 用途 |
| --- | --- |
| `funds.json` | 自选基金、份额、成本价、选中项及部分界面设置 |
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
2. 构建 macOS 通用应用与 DMG。
3. 使用 `codesign --verify --deep --strict` 校验签名。
4. 上传应用与安装包作为构建产物。
5. 对 `v*` 标签创建附带 DMG 的 **草稿 Release**，供维护者检查后发布。

配置仓库 Secrets `MACOS_SIGNING_IDENTITY`、`MACOS_CERT_P12`（base64）和 `MACOS_CERT_PASSWORD` 后，CI 会导入证书并签名；未配置时使用 ad-hoc 签名。当前工作流没有配置公证凭据的执行步骤，发布公证版本时还需补齐该流程。
