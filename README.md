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

Pane B に移動せずに登録する場合は、Pane A で `panecom register` を実行します。現在のタブ／ウィンドウ内で、矢印キー（または `h`/`j`/`k`/`l`）を使って画面上の上下左右にある pane を選び、Enter、続いて role 名を入力してください。role 入力中は `Tab` で `developer` / `reviewer` を補完できます。例えば `rev`→`Tab` で `reviewer` になり、空欄からの `Tab` は候補を順に表示します。任意の role 名も入力できます。端に着いても選択は飛びません。選んだ pane は Zellij では一時的に色付き、tmux では mark 表示され、画面のプレビューも表示されます。選んだ pane の作業ディレクトリからプロジェクトを判定します。作業ディレクトリを取得できない場合は、プロジェクトのパスも入力します。`Esc` または `q` で pane 選択を、role 入力中は `Esc` で登録を中止できます。対象 pane にコマンドは送られませんが、対象プロジェクトの `.panecom/` に登録状態を書き込み（存在しなければ作成）、pane のタイトルを `panecom:<role>` に変更します。別プロジェクトの pane を選ぶと、そのプロジェクトに登録され、呼び出し元からは `panecom link` 後に `<project>/<role>` で参照できます。

Zellij の色は選択画面を閉じると標準色に戻ります。選択前に `set-pane-color` で個別に設定した色は復元されません。

```text
$ panecom register
Select a pane  arrows/hjkl: move  Enter: select  Esc/q: cancel
Tab/window: app  (0)

  [app] terminal_1  claude  /path/to/app
> [app] terminal_2  codex   /path/to/app

Preview:
...
Role name (Tab: developer/reviewer): rev<Tab>
reviewer -> codex (/path/to/app)
```

> 両方の pane は **同じセッション内**、**同じプロジェクト** から register してください。これが同じ namespace に所属する条件です。プロジェクトは register 時の作業ディレクトリから遡って判定されるので、リポジトリ内のどのディレクトリで register しても構いません。

登録内容は `panecom whoami` で確認できます。同じプロジェクト内で作業ディレクトリを移動しても、登録時に保存された namespace を表示します。別プロジェクトへ移動し、state store も変わった場合は登録が見つからずエラーになります。従来の role だけの出力をスクリプトで使う場合は `panecom whoami --role` に変更してください。

AI エージェントに「あなたがレビュワーね」「developer として進めて」のように役割を指定した場合は、配布された panecom skill に従い、エージェント自身が `panecom register reviewer` / `panecom register developer` を実行して登録を確認します。developer は実装後、commit 前に reviewer へレビューを依頼し、reviewer はテストと動作確認を含めて確認します。作業中の reviewer はおおむね 1 時間ごとに developer の画面を確認します（AI が停止中に起動するタイマーではありません）。別プロジェクトの相手との協業を依頼した場合も、パスが分かればエージェント自身が必要な `link` を設定できます。

```text
$ panecom whoami
role: reviewer
pane: terminal_2
namespace: /path/to/app
```

以前のバージョンで登録した pane は、サブディレクトリを namespace として記録したままの場合があります。`panecom list` は同じ state store にある生存中の旧登録を検知し、対象 pane で実行する `panecom register <role>`、または同じタブの別 pane から選ぶ `panecom register` を案内します。role の解決に失敗した場合も同じ案内を表示します。検知だけでは登録を変更しません。別の state store に残った登録、および `.git` や `.panecom` のマーカーが無く同じプロジェクトと判定できない登録は検知対象外です。

4. 通信する:

```bash
# Pane A から
panecom resolve reviewer                    # → terminal_N (Zellij) / %N (tmux)
panecom tell reviewer "コードをレビューしてください"  # Pane B のエージェントに送られる
panecom dump reviewer                       # Pane B の現在画面を取得
panecom focus reviewer                      # Pane B に移動する
```

> **注意**: `tell` / `send` は対象 pane に文字列 + Enter を注入します。対象が素のシェルの場合、送信内容がコマンドとして実行されます。エージェントの入力には `tell` を使ってください。

### tell と send の使い分け

相手が AI エージェントなら `tell`、pane をそのまま操作したいなら `send` を使います。

| | `tell` | `send` |
|---|---|---|
| 相手 | role を持つ AI エージェント | 任意の pane（シェル、TUI 等） |
| 送る内容 | 1 行目に `[panecom:from=<自分の role>]` が付く | 与えた文字列をそのまま |
| 自分の登録 | 必須（未登録ならエラー） | 不要 |

```bash
$ panecom tell reviewer "レビューをお願いします"
# 相手の pane には次が届く:
#   [panecom:from=developer]
#   レビューをお願いします
```

別プロジェクトの相手（`<alias>/<role>`）に送ると、ヘッダ 2 行目に自分のプロジェクトのパスが入ります。role 名は namespace の中でしか通用しないため、これが無いと相手は返信先を解決できません。

```
[panecom:from=developer]
[panecom:project=/path/to/my project]
レビューをお願いします
```

受け取った側は `panecom link <alias> "/path/to/my project"` してから `panecom tell <alias>/developer` で返します。role 名だけで返すと、自分のプロジェクトの同名 role に送ってしまいます。

ヘッダは 1 行に 1 つの値を置き、`[panecom:<キー>=` と行末の `]` の間が値です。パスに空白が含まれても読み取れるよう、空白では区切りません。自分のプロジェクトを特定できない場合、`tell` は送信せずエラーになります。

`send` は汎用のプリミティブなのでヘッダを付けません。シェルや TUI を相手にしたときに混ざると壊れるためです。逆にエージェント宛に `send` を使うと送信者が付かず、相手は返信先を判断できません。

### 配信確認（tell / send 共通）

相手が入力を受け付けない状態（詳細表示、ページャ、確認ダイアログなど）だと、送った文字列は黙って捨てられます。`tell` / `send` は送信前後に対象 pane の画面を取得して配信を確認し、確認できない場合は非ゼロで終了します（デフォルト ON）。

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
| `role 'X' points to stale pane` | 相手の pane が閉じられた | 相手側で再度 `panecom register X`（登録自体はこの時点で削除される） |

## コマンド一覧

| コマンド | 説明 |
|---|---|
| `[--mux zellij\|tmux]` | マルチプレクサを明示指定（省略時は自動検出） |
| `register` | pane を画面で選び、role 名を入力して登録 |
| `register <role>` | 現在の pane を role として登録 |
| `whoami [--role]` | 現在の pane の role、pane ID、登録時の namespace を表示（`--role` は role のみ） |
| `list [--all \| <project>] [--stale]` | role 一覧を表示（自分 / リンク先 / 全部）。stale な role は既定で非表示、`--stale` で表示 |
| `cleanup [--all] [-n]` | pane が消えた role の登録と、終了したセッションの state を削除（`--all` はリンク先の role も、`-n` は dry run） |
| `link [-f] <project> <path>` | 別プロジェクトをリンク（引数なしでリンク一覧） |
| `unlink <project>` | リンクを削除 |
| `tell [--no-verify] [--verify-timeout N] <role> <msg>` | エージェントにメッセージを送る。1 行目に `[panecom:from=<自分の role>]` が付く（配信確認あり） |
| `resolve <role>` | role → pane ID を解決 |
| `focus <role>` | 対象 pane にフォーカスを移動 |
| `open [-d right\|down] <role>` | 新しい pane を開いて role を登録 |
| `dump [--full] <role>` | 対象 pane の画面を取得 |
| `send [--no-verify] [--verify-timeout N] <role> <msg>` | 対象 pane に文字列をそのまま打ち込む（配信確認あり） |
| `share [--full] <role>` | 自分の画面を相手に共有 |
| `profile [-f] <name>` | プロファイルから環境を構築 |
| `skill install` | agent と scope を選んで skill を適用 |
| `skill list` | panecom が配布を記録した skill の一覧 |
| `skill uninstall` | 記録済みの配布先を選んで解除 |
| `skill sync [--check] <file>...` | 指定ファイルの panecom skill ブロックを再生成（`--check` は書き込まず差分の有無だけ報告） |
| `update` | 最新版に自己更新 |
| `--version` | バージョンを表示 |

## オプション詳細

### dump / share

- `--full` — 現在の画面だけでなく、スクロールバック全体を取得する

### open

- `-d right|down` — pane を開く方向（デフォルト: 未指定で Zellij の自動配置）

### send / tell

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

プロジェクトルートの `.panecom.yaml` にプロファイルを定義し、開発環境を一発で立ち上げます。このファイルだけを新しいリポジトリへコピーすればよく、セッション情報が入る `.panecom/` をコピーする必要はありません。

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

`panecom profile dev` 後、developer の Claude から `panecom send terminal "make test"` でターミナルにコマンドを送り、`panecom dump terminal` で結果を読めます。

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
| 高 | `<プロジェクトルート>/.panecom.yaml` |
| 代替 | 従来の `.panecom/config.yaml` または `.panecom/config.yml`（新しいファイルが無い場合のみ） |
| 低 | `~/.config/panecom/config.yaml` または `config.yml` |

`XDG_CONFIG_HOME` が設定されている場合、グローバル設定は `$XDG_CONFIG_HOME/panecom/` を参照します。

同名プロファイルはプロジェクト側が上書きします（フィールド単位のマージではなくプロファイル全体の置き換え）。既存の `.panecom/config.yaml` は引き続き読めます。移行する場合は、設定ファイルだけをプロジェクトルートの `.panecom.yaml` へ移動してください。

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
  terminal     terminal_14

$ panecom tell mcp/reviewer "schema 確認お願いします"   # エージェント宛は tell
$ panecom send mcp/terminal "git status"                # シェル pane 宛は send
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

### stale な role の扱い

pane が閉じられた role は stale になります。マルチプレクサを再起動すると pane ID が一斉に振り直されるため、その namespace の role がまとめて stale になることがあります。`list` は stale を既定で隠し、件数だけ表示します。

```bash
$ panecom list
/path/to/project
  d            terminal_516
  (4 stale entries hidden: panecom list --stale, panecom cleanup)

$ panecom list --stale    # 中身を確認
$ panecom cleanup -n      # 消す対象を確認（削除しない）
$ panecom cleanup         # 削除
```

`tell` / `send` / `resolve` / `dump` / `share` / `focus` は stale な role に当たった時点でその登録を自動的に削除します。`cleanup` は誰も触らないまま残った分をまとめて片付けるためのものです。

pane の生存確認そのものに失敗した role は `(unknown)` と表示され、削除されません。マルチプレクサが応答しないだけで全 role が消えるのを防ぐためです。

### 終了したセッションの state

state はセッション単位で `.panecom/sessions/<hash>/` に置かれます。マルチプレクサを再起動すると、古いセッションのディレクトリがそのまま残ります。`list` は現在のセッションしか見ないので、この分は一覧に現れないまま溜まります。

`cleanup` は role の掃除に加えて、実行中でないセッションのディレクトリを削除します。

```bash
$ panecom cleanup -n
/path/to/project
  would remove developer    terminal_268
  would remove session lucky-drum
  would remove session quiet-zebra
4 stale entries, 2 dead sessions would be removed
```

- Zellij の EXITED セッション（attach で resurrect できる状態）は「実行中でない」として扱います。resurrect しても pane ID は振り直されるため、古いマッピングは使えません
- 現在のセッションは、マルチプレクサの一覧に出てこなくても削除しません
- セッション一覧の取得に失敗した場合は、セッションの削除を丸ごと見送ります
- 別のマルチプレクサのセッション（tmux 下で作られた state を Zellij から見た場合など）は、生存確認ができないため対象外です
- 対象は自分のプロジェクトの store のみです。`--all` はリンク先の role を対象に加えますが、リンク先の store のセッション state には触れません

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
.panecom.yaml            # 共有できるプロファイル設定（プロジェクトルート）
.panecom/
├── externals/           # 別プロジェクトへのリンク（セッションを跨いで永続）
│   └── <anchor-hash>/
│       └── <alias>      # 連携先のプロジェクトルート
├── sessions/            # ランタイム状態（自動生成）
│   └── <session-hash>/
│       ├── panes/
│       ├── namespaces/
│       └── .session
```

`sessions/` はランタイムデータです。`.gitignore` に追加してください:

```
.panecom/sessions/
# v3.0.0 より前の exec の残骸。新規には作られないが、
# 既存データにはコマンドとその出力が含まれる
.panecom/exec/
.panecom/exec.lock
```

v3.0.0 より前のバージョンは `.panecom/exec/` と `.panecom/exec.lock` も作成していました。`exec` コマンドの削除に伴い新規には作られませんが、**既存のデータは残ります**。実行したコマンドと stdout / stderr が入っているため、ignore は外さず、不要なら削除してください。

## Safety

panecom は対象 pane に対して **直接的な文字列注入と任意コマンド実行** を行います。

- **`send` / `tell`** は対象 pane に文字列と Enter を注入します。対象がシェルの場合、送信内容はコマンドとして実行されます。メッセージ API ではありません。呼び出し側のサンドボックスや権限管理の外で動作します
- **`dump` / `share`** は対象 pane の画面内容を転送します。画面に表示されている機密情報が含まれる可能性があります

**信頼境界**: 登録済み role への send / tell は、対象 pane のユーザー権限で任意の操作が可能です。信頼できる role にのみ送信してください。マルチエージェント連携の用途ではこれは想定どおりの動作です。

## AI エージェント向け Skill

インストール済みの panecom から Skill を配布できます。`panecom skill install` は `~/.claude` と `~/.codex` のディレクトリの有無を表示し、Claude / Codex と user（全プロジェクト）/ project（現在のプロジェクト）の組み合わせをチェック式の画面で選べます。上下矢印（または `j`/`k`）で移動、Space でチェックし、最後の `Install selected` に移動して Enter で適用します。`Tab` で適用ボタンへ移動、`Esc` または `q` で中止できます。両 agent に別々の scope を指定することも、同じ agent を user と project の両方に入れることもできます。ディレクトリの有無は agent 本体のインストール確認ではありません。マルチプレクサ外でも実行できますが、対話用の端末が必要です。

```bash
panecom skill install
# [x] Claude / User (all projects)
# [ ] Claude / Project (current repository)
# [ ] Codex  / User (all projects)
# [x] Codex  / Project (current repository)
# > [ Install selected (2) ]
```

### skill を変更したとき

`skill install` は対話 UI なので、リポジトリに配布済みのファイル（このリポジトリの `AGENTS.md` 等）を機械的に追従させる用途には使えません。`skill sync` を使います。

```bash
panecom skill sync AGENTS.md           # ブロックを再生成
panecom skill sync --check AGENTS.md   # ずれていれば非ゼロ終了（CI 向け）
```

マーカー（`<!-- panecom-skill:start -->` 〜 `<!-- panecom-skill:end -->`）の外側は書き換えません。マーカーが無いファイルには末尾に追記します。

このリポジトリでは CI の `skill-sync` ジョブが `--check` を実行し、`.claude/skills/panecom.md` を変更して `AGENTS.md` を再生成し忘れると fail します。

選択した配布先は `~/.config/panecom/skill-installs.json` に記録されます。次のコマンドで確認・解除できます。`list` は従来のスクリプトなどで入れた配布先も、現在の user/project で見つかる範囲は `untracked` と表示します。同じ内容の既存配布を選んで install すると管理対象として引き継ぎ、uninstall 時は元からあったファイルを残します。独自に編集された既存ファイルは install 前の内容と権限を保存して復元します。Claude の既存ファイルは install 時に全文を置き換えるため、選択画面にその旨を表示します。配布先が symlink の場合は通常ファイルに置き換わり、uninstall でも symlink は復元されません。配布先がすでに削除されている場合は追跡記録だけを解除します。uninstall は後から skill 本体が編集されていた場合、保護のため中止します。

```bash
panecom skill list
panecom skill uninstall
```

install は実行中の panecom バイナリに同梱された skill を使います。最新版にしたいときは、先に `panecom update` または `go install .` でバイナリを更新し、`panecom skill install` を再実行してください。エージェントが新しい指示を読むには、新しいセッションを開始してください。

リポジトリの `AGENTS.md` も再生成するときは、従来どおりソースツリーで `scripts/deploy-skill.sh` を実行します。

```bash
./scripts/deploy-skill.sh
```

配布先は次のとおりです:

| 対象 | パス | 更新方法 |
|---|---|---|
| Claude Code (user) | `~/.claude/skills/panecom.md` | `skill install` / スクリプト |
| Claude Code (project) | `<project>/.claude/skills/panecom.md` | `skill install` |
| Codex (project) | `<project>/AGENTS.md` | `skill install` / ソースツリーのスクリプト |
| Codex (user) | `~/.codex/instructions.md` | `skill install` / スクリプト |

再実行は冪等です（マーカーで囲んだセクションを上書き）。

## ライセンス

MIT License — [LICENSE](LICENSE) を参照してください。
