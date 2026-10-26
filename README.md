# panecom

ターミナルマルチプレクサの pane 間通信を role 名で抽象化する CLI ツール。

AI エージェント同士（Claude、Codex など）が pane ID を意識せずに通信できるようにします。人と AI が同じターミナルで共同作業するユースケースもサポートします。

## 対応マルチプレクサ

| マルチプレクサ | 状態 |
|---|---|
| Zellij 0.45+ | サポート済み（`list-panes --json`、`new-pane --tab-id` を使用） |
| tmux 3.2+ | サポート済み（`switch-client`、`capture-pane -p` を使用） |

### マルチプレクサの選択

検出は以下の優先順で行われます:

| 優先度 | 方法 | 例 |
|---|---|---|
| 1 | `--mux` フラグ | `panecom --mux tmux register developer` |
| 2 | `PANECOM_MUX` 環境変数 | `PANECOM_MUX=tmux panecom register developer` |
| 3 | 自動検出: `ZELLIJ_SESSION_NAME` | Zellij セッション内で自動選択 |
| 4 | 自動検出: `TMUX` | tmux セッション内で自動選択 |

両方の環境変数がある場合、自動検出では Zellij が優先されます。明示的に `--mux` または `PANECOM_MUX` で指定してください。

## インストール

### ソースからビルド

```bash
git clone https://github.com/konono/panecom.git
cd panecom
go build -o bin/panecom .
```

ビルドしたバイナリを PATH の通った場所にコピーしてください。

### リリースバイナリ

リリースが公開されると、[Releases](https://github.com/konono/panecom/releases) ページからプラットフォームに合ったバイナリをダウンロードできます。

### go install

```bash
go install github.com/konono/panecom@latest
```

## クイックスタート

### 前提条件

- Go 1.27+（ビルド時のみ）
- Zellij 0.45+ または tmux 3.2+
- Linux (amd64/arm64) または macOS (amd64/arm64)

### 最小手順

1. マルチプレクサのセッションを起動します:

```bash
zellij    # Zellij の場合
# または
tmux      # tmux の場合
```

2. Pane A で developer として登録:

```bash
panecom register developer
```

3. 別の pane（Pane B）を開き、reviewer として登録します。Pane B では Claude や Codex などのエージェントを起動してください:

```bash
panecom register reviewer
codex  # または claude 等
```

Pane B に移動せずに登録する場合は、Pane A で `panecom register` を実行します。上下キー（または `j`/`k`）で pane を選び、画面のプレビューを確認して Enter、続いて role 名を入力してください。選んだ pane の作業ディレクトリからプロジェクトを判定します。作業ディレクトリを取得できない場合は、プロジェクトのパスも入力します。`q` で選択を中止できます。対象 pane にコマンドは送られません。

```text
$ panecom register
Select a pane  ↑/↓ or j/k: move  Enter: select  q: cancel

  [app] claude  /path/to/app
> [mcp] codex   /path/to/mcp

Preview:
...
Role name: m
m -> codex (/path/to/mcp)
```

> 両方の pane は **同じセッション内**、**同じプロジェクト** から register してください。これが同じ namespace に所属する条件です。プロジェクトは register 時の作業ディレクトリから遡って判定されるので、リポジトリ内のどのディレクトリで register しても構いません。

4. 通信する:

```bash
# Pane A から
panecom resolve reviewer                    # → terminal_N (Zellij) / %N (tmux)
panecom send reviewer "コードをレビューしてください"  # Pane B のエージェントに送られる
panecom dump reviewer                       # Pane B の現在画面を取得
```

> **注意**: `send` は対象 pane に文字列 + Enter を注入します。対象が素のシェルの場合、送信内容がコマンドとして実行されます。エージェントの入力として使うか、`exec` コマンドを使ってください。

### send の配信確認

相手が入力を受け付けない状態（詳細表示、ページャ、確認ダイアログなど）だと、送った文字列は黙って捨てられます。`send` は送信前後に対象 pane の画面を取得して配信を確認し、確認できない場合は非ゼロで終了します（デフォルト ON）。

| exit code | 意味 |
|---|---|
| 0 | 画面が変化し、入力欄にメッセージが残っていない（受理とみなす） |
| 1 | role 未解決、pane 消失などのエラー |
| 3 | 画面が全く変化しなかった（未受理の疑い） |
| 4 | 入力欄にメッセージが残っている（Enter が効いていない） |

非ゼロ終了時は相手画面の末尾 20 行が stderr に出力されます。確認をスキップするには `--no-verify`、描画が遅い相手には `--verify-timeout 10` を指定します。

なお exit 0 は「画面が動いた」ことの確認であり、相手が依頼内容を理解した保証ではありません。exit 4 の判定は「メッセージが画面の末尾に残っているか」で行うため（折り返しは考慮済み）、入力欄の下にヒント行を描画する UI では検出できないことがあります。検出漏れ側に倒しているのは、受理済みのメッセージを失敗と誤判定して二重送信することを避けるためです。

### よくあるエラー

| エラー | 原因 | 対処 |
|---|---|---|
| `not running inside a supported multiplexer` | Zellij/tmux 外で実行 | マルチプレクサのセッション内で実行する |
| `unsupported multiplexer: X` | 未対応の `--mux` 値 | `zellij` または `tmux` を指定 |
| `ZELLIJ_PANE_ID not set` / `TMUX_PANE not set` | pane ID が取得できない | マルチプレクサの terminal pane 内で実行する |
| `role 'X' is not registered in namespace <path>` | 相手が未登録 or 別 namespace | メッセージの `<path>` が相手の register 先と一致するか確認し、同じプロジェクトから register し直す |
| `role 'X' points to stale pane` | 相手の pane が閉じられた | 相手側で再度 `panecom register X` |

## コマンド一覧

| コマンド | 説明 |
|---|---|
| `[--mux zellij\|tmux]` | マルチプレクサを明示指定（省略時は自動検出） |
| `register` | pane を画面で選び、role 名を入力して登録 |
| `register <role>` | 現在の pane を role として登録 |
| `whoami` | 現在の pane の role を表示 |
| `list [--all \| <project>]` | role 一覧を表示（自分 / リンク先 / 全部） |
| `link [-f] <project> <path>` | 別プロジェクトをリンク（引数なしでリンク一覧） |
| `unlink <project>` | リンクを削除 |
| `resolve <role>` | role → pane ID を解決 |
| `open [-d right\|down] <role>` | 新しい pane を開いて role を登録 |
| `dump [--full] <role>` | 対象 pane の画面を取得 |
| `send [--no-verify] [--verify-timeout N] <role> <msg>` | 対象 pane にメッセージを送信（配信確認あり） |
| `share [--full] <role>` | 自分の画面を相手に共有 |
| `exec [--timeout N] <role> <cmd>` | 対象 pane でコマンドを実行し結果を返す |
| `profile [-f] <name>` | プロファイルから環境を構築 |
| `update` | 最新版に自己更新 |
| `--version` | バージョンを表示 |

## オプション詳細

### dump / share

- `--full` — 現在の画面だけでなく、スクロールバック全体を取得する

### open

- `-d right|down` — pane を開く方向（デフォルト: 未指定で Zellij の自動配置）

### exec

- `--timeout N` — コマンド完了待ちのタイムアウト秒数（デフォルト: `30`）。タイムアウトは呼び出し側の待機を止めるだけで、**対象 pane で実行中のコマンドは停止しません**

ターミナルには `PANECOM_TOKEN=<token> panecom exec -- $'<command>'` と表示されます。`PANECOM_TOKEN` は request の一意性を保証するためのプロトコル要素で、省略できません。コマンドは先頭スペース付きで送信されるため、シェルの `HIST_IGNORE_SPACE` が有効であればヒストリに記録されません。

```bash
# ~/.zshrc に追加（推奨）
setopt HIST_IGNORE_SPACE

# bash の場合
export HISTCONTROL=ignorespace
```

### profile

- `-f` / `--focus` — 作成したタブにフォーカスを移す（デフォルト: 元のタブに戻る）

### update

- GitHub Releases から最新版をダウンロードし、実行中のバイナリを自動で置き換えます
- 現在のバージョンが最新の場合は何もしません

```bash
panecom update           # 最新版に更新
panecom --version        # 現在のバージョンを確認
```

### role 名の制約

`[A-Za-z0-9][A-Za-z0-9._-]*` にマッチする必要があります。`/` や `..` はパストラバーサル防止のため使用できません。

## プロファイル

`.panecom/config.yaml` にプロファイルを定義し、開発環境を一発で立ち上げます。

```yaml
profiles:
  review:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: reviewer
        cmd: codex
      - role: terminal
        direction: down
```

```bash
panecom profile review        # review 環境を立ち上げ
panecom profile -f review     # 作成した pane にフォーカスを移す
```

### ユースケース例

**AI ペアプログラミング**: Claude と Codex で相互レビュー

```yaml
profiles:
  pair:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: reviewer
        cmd: codex
```

**共有ターミナル付き開発**: AI エージェント + コマンド実行用 pane

```yaml
profiles:
  dev:
    panes:
      - role: developer
        cmd: claude
        foreground: true
      - role: terminal
        direction: down
```

`panecom profile dev` 後、developer の Claude から `panecom exec terminal "make test"` でターミナルにコマンドを送れます。

### プロファイルの各フィールド

| フィールド | 説明 | デフォルト |
|---|---|---|
| `role` | pane に割り当てる role 名（必須） | — |
| `cmd` | pane で実行するコマンド | なし（素のシェル） |
| `direction` | pane の分割方向 (`right` / `down`) | `right` |
| `foreground` | `-f` 指定時にこの pane にフォーカス | `false` |

プロファイル直下には `external_projects` も書けます（後述の「別プロジェクトとの連携」参照）。

### 設定ファイルの優先順位

| 優先度 | パス |
|---|---|
| 高 | `.panecom/config.yaml` または `.panecom/config.yml`（プロジェクトルートから親を walk-up） |
| 低 | `~/.config/panecom/config.yaml` または `config.yml` |

`XDG_CONFIG_HOME` が設定されている場合、グローバル設定は `$XDG_CONFIG_HOME/panecom/` を参照します。

同名プロファイルはプロジェクト側が上書きします（フィールド単位のマージではなくプロファイル全体の置き換え）。

## 設計

panecom はメッセージングサーバーではありません。daemon も持ちません。

```
developer ─────→ pane_2
reviewer  ─────→ pane_5
                    │
                    ▼
              Zellij / tmux
```

マルチプレクサの pane ID を安定した role 名へ抽象化する薄い wrapper です。

### Namespace

role はセッション + **プロジェクトルート**で namespace が決まります。異なるプロジェクトの role は互いに干渉しません。

プロジェクトルート（anchor）は register 時の作業ディレクトリから親を遡って決定されます:

1. 最初に見つかった `.git`（通常の clone はディレクトリ、worktree / submodule はファイル）
2. `.git` が無ければ、最初に見つかった `.panecom/`
3. いずれも無ければ作業ディレクトリ自身

`.git` を全階層で優先するため、サブディレクトリに `.panecom/` が残っていても同一リポジトリが分裂しません。また**ホームディレクトリとファイルシステムルートは project root として扱いません** — catch-all の `~/.panecom` は共有の状態置き場であってプロジェクトではなく、そこを anchor にすると無関係なプロジェクトが 1 つの namespace に合流するためです。

この仕組みにより、register 後に作業ディレクトリがリポジトリ内で移動しても namespace は変わりません。

> **v0.3.0 からの変更**: 以前は作業ディレクトリそのものが namespace を決めていました。アップグレード後は namespace のハッシュが変わります。登録済みの pane は `panes/<id>` に namespace を直接記録しているため解決は維持されますが、**アップグレード前に登録した pane と新しく登録した pane を相互に見せるには、古い側で `panecom register` をやり直してください**。

### 別プロジェクトとの連携

別プロジェクトをリンクすると、その role を `<project>/<role>` で呼べます。role 名に `/` は使えないので、最後の `/` の手前がプロジェクト、後ろが role です。修飾しない `<role>` は従来どおり自分の namespace を指します。

```bash
$ panecom link mcp ../rh_sc_mcp
mcp -> /path/to/rh_sc_mcp
  developer    terminal_12
  reviewer     terminal_13

$ panecom send mcp/reviewer "schema 確認お願いします"
$ panecom exec mcp/developer "git status"
```

`link` は相手の role をその場で検証して一覧表示します。相手がまだ register していなければ失敗するので、送信時まで気づかないということがありません。相手の起動前に張っておきたい場合は `-f` を使います。

```bash
$ panecom link mcp ../rh_sc_mcp
panecom: no roles registered in /path/to/rh_sc_mcp (session: ope)
         pass --force to link anyway
```

一覧は `list` で確認できます。

```bash
$ panecom list            # 自分の namespace
$ panecom list mcp        # リンク先
$ panecom list --all      # 自分 + 全リンク先
$ panecom link            # リンクの一覧（role は引かない）
$ panecom unlink mcp
```

#### 性質

- **リンクが保存するのはパス 1 本だけ**です。role は呼び出しのたびに相手の状態を直接読むので、相手が register し直しても張り直しは不要です
- **片方向**です。相手の状態には何も書き込まれません。往復させるには相手側でも `panecom link` を張ります
- **セッションを跨いで残ります**。`link` はパスを記録するだけなので、Zellij / tmux のセッションを作り直しても有効です
- 渡したパスは**プロジェクトルートに正規化**されます。`../rh_sc_mcp/src` のようにサブディレクトリを指定しても、上記 anchor の規則で親を遡って解決します
- 送信先のペインは**同じマルチプレクサセッション内**にある必要があります。pane ID がセッション内でしか有効でないためです

#### 繰り返す連携を config に残す

毎回 `link` するのが面倒になったら、プロファイルに宣言できます。`panecom profile <name>` の起動時にリンクが作られます。

```yaml
profiles:
  dev:
    external_projects:
      mcp: ../rh_sc_mcp
    panes:
      - role: developer
        cmd: 'claude --allow-dangerously-skip-permissions'
        foreground: true
      - role: reviewer
        cmd: 'codex --yolo'
```

パスは config のあるプロジェクトルートからの相対として解決されます。コミットして共有する場合は、絶対パスではなく相対パスを使ってください。起動時点では相手がまだ動いていないのが普通なので、ここでの materialize は role の有無を検証しません（`link -f` 相当）。

`.panecom/externals/` 配下の実体はマシン固有の絶対パスなので `.gitignore` 対象です。共有したいリンクは config に書きます。

### State

状態ディレクトリは以下の優先順で決定されます:

1. `PANECOM_STATE_DIR` 環境変数（設定されていれば最優先）
2. 上記の anchor から親ディレクトリを遡って見つかった既存の `.panecom/`
3. いずれもなければ `<anchor>/.panecom/` を新規作成

探索を cwd ではなく anchor から始めるため、サブディレクトリに残った `.panecom/` がプロジェクト本来の状態ディレクトリを隠すことはありません。

```
.panecom/
├── config.yaml          # プロファイル設定
├── externals/           # 別プロジェクトへのリンク（セッションを跨いで永続）
│   └── <anchor-hash>/
│       └── <alias>      # 連携先のプロジェクトルート
├── sessions/            # ランタイム状態（自動生成）
│   └── <session-hash>/
│       ├── panes/
│       ├── namespaces/
│       └── .session
├── exec.lock            # exec 実行中ロック（同時実行防止、atomic）
└── exec/                # exec のランタイム状態
    ├── current -> <token> # 最新 exec へのシンボリックリンク
    └── <token>/          # request ごとのデータ
        ├── command       # 実行中のコマンド
        ├── stdout        # stdout（リアルタイム書き込み）
        ├── stderr        # stderr（リアルタイム書き込み）
        └── exitcode      # 終了コード（完了時に作成）
```

`sessions/` と `exec/` はランタイムデータです。`.gitignore` に追加してください:

```
.panecom/sessions/
.panecom/exec/
.panecom/exec.lock
```

exec は同時に1つだけ実行できます（`exec.lock` で排他制御）。実行中に別の exec を開始するとエラーになります。
完了済みの古い exec データは次回の exec 開始時に自動削除されます。
実行中の進捗は `cat .panecom/exec/current/stdout` で確認できます。

## Safety

panecom は対象 pane に対して **直接的な文字列注入と任意コマンド実行** を行います。

- **`send`** は対象 pane に文字列と Enter を注入します。対象がシェルの場合、送信内容はコマンドとして実行されます。メッセージ API ではありません
- **`exec`** は対象 pane 上で `sh -c` 経由のコマンド実行です。呼び出し側のサンドボックスや権限管理の外で動作します
- **`exec --timeout`** のタイムアウトは呼び出し側の待機を止めるだけです。対象 pane で実行中のコマンドは停止しません
- **`dump` / `share`** は対象 pane の画面内容を転送します。画面に表示されている機密情報が含まれる可能性があります

**信頼境界**: 登録済み role への send / exec は、対象 pane のユーザー権限で任意の操作が可能です。信頼できる role にのみ送信してください。マルチエージェント連携の用途ではこれは想定どおりの動作です。

## AI エージェント向け Skill

`scripts/deploy-skill.sh` で Claude Code と Codex の両方に panecom の Skill を配布できます。

```bash
./scripts/deploy-skill.sh
```

このスクリプトは以下のファイルを作成・更新します:

| 対象 | パス |
|---|---|
| Claude Code (グローバル) | `~/.claude/skills/panecom.md` |
| Claude Code (プロジェクト) | `.claude/skills/panecom.md` |
| Codex (プロジェクト) | `AGENTS.md` |
| Codex (グローバル) | `~/.codex/instructions.md` |

再実行は冪等です（マーカーで囲んだセクションを上書き）。

## ライセンス

MIT License — [LICENSE](LICENSE) を参照してください。
