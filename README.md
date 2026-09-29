# panecom

ターミナルマルチプレクサの pane 間通信を role 名で抽象化する CLI ツール。

AI エージェント同士（Claude、Codex など）が pane ID を意識せずに通信できるようにします。人と AI が同じターミナルで共同作業するユースケースもサポートします。

## 対応マルチプレクサ

| マルチプレクサ | 状態 |
|---|---|
| Zellij 0.45+ | サポート済み（`list-panes --json`、`new-pane --tab-id` を使用） |
| tmux | 計画中（全 API に同等コマンドあり、未実装） |

## インストール

### ソースからビルド

```bash
git clone https://github.com/konono/panecom.git
cd panecom
go build -o bin/panecom .
```

ビルドしたバイナリを PATH の通った場所にコピーしてください。

### リリースバイナリ（初回リリース後）

リリースが公開されると、[Releases](https://github.com/konono/panecom/releases) ページからプラットフォームに合ったバイナリをダウンロードできます。

### go install（初回リリース後）

```bash
go install github.com/konono/panecom@latest
```

## クイックスタート

### 前提条件

- Go 1.27+（ビルド時のみ）
- Zellij 0.45+（`list-panes --json`、`--tab-id` オプションが必要）
- Linux (amd64/arm64) または macOS (amd64/arm64)

### 最小手順

1. Zellij セッションを起動します:

```bash
zellij
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

> 両方の pane は **同じ Zellij セッション内**、**同じディレクトリ** から register してください。これが同じ namespace に所属する条件です。

4. 通信する:

```bash
# Pane A から
panecom resolve reviewer                    # → terminal_N
panecom send reviewer "コードをレビューしてください"  # Pane B のエージェントに送られる
panecom dump reviewer                       # Pane B の現在画面を取得
```

> **注意**: `send` は対象 pane に文字列 + Enter を注入します。対象が素のシェルの場合、送信内容がコマンドとして実行されます。エージェントの入力として使うか、`exec` コマンドを使ってください。

### よくあるエラー

| エラー | 原因 | 対処 |
|---|---|---|
| `ZELLIJ_SESSION_NAME not set` | Zellij 外で実行 | Zellij セッション内で実行する |
| `ZELLIJ_PANE_ID not set` | Zellij が起動した pane 外で実行 | Zellij の terminal pane 内で実行する。pane を閉じて開き直す |
| `role 'X' is not registered` | 相手が未登録 or 別 namespace | 同じディレクトリから register する |
| `role 'X' points to stale pane` | 相手の pane が閉じられた | 相手側で再度 `panecom register X` |

## コマンド一覧

| コマンド | 説明 |
|---|---|
| `register <role>` | 現在の pane を role として登録 |
| `whoami` | 現在の pane の role を表示 |
| `resolve <role>` | role → pane ID を解決 |
| `open [-d right\|down] <role>` | 新しい pane を開いて role を登録 |
| `dump [--full] <role>` | 対象 pane の画面を取得 |
| `send <role> <msg>` | 対象 pane にメッセージを送信 |
| `share [--full] <role>` | 自分の画面を相手に共有 |
| `exec [--timeout N] <role> <cmd>` | 対象 pane でコマンドを実行し結果を返す |
| `profile [-f] <name>` | プロファイルから環境を構築 |

## オプション詳細

### dump / share

- `--full` — 現在の画面だけでなく、スクロールバック全体を取得する

### open

- `-d right|down` — pane を開く方向（デフォルト: 未指定で Zellij の自動配置）

### exec

- `--timeout N` — コマンド完了待ちのタイムアウト秒数（デフォルト: `30`）。タイムアウトは呼び出し側の待機を止めるだけで、**対象 pane で実行中のコマンドは停止しません**

### profile

- `-f` / `--focus` — 作成したタブにフォーカスを移す（デフォルト: 元のタブに戻る）

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
panecom profile review
```

### プロファイルの各フィールド

| フィールド | 説明 | デフォルト |
|---|---|---|
| `role` | pane に割り当てる role 名（必須） | — |
| `cmd` | pane で実行するコマンド | なし（素のシェル） |
| `direction` | pane の分割方向 (`right` / `down`) | `right` |
| `foreground` | `-f` 指定時にこの pane にフォーカス | `false` |

### 設定ファイルの優先順位

| 優先度 | パス |
|---|---|
| 高 | `.panecom/config.yaml` または `.panecom/config.yml`（cwd から親を walk-up） |
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
              Zellij (現在)
              tmux  (計画中)
```

マルチプレクサの pane ID を安定した role 名へ抽象化する薄い wrapper です。

### Namespace

role はセッション + register 時の作業ディレクトリで namespace が決まります。異なるプロジェクトの role は互いに干渉しません。

### State

状態ディレクトリは以下の優先順で決定されます:

1. `PANECOM_STATE_DIR` 環境変数（設定されていれば最優先）
2. cwd から親ディレクトリを遡って見つかった既存の `.panecom/`
3. いずれもなければ `cwd/.panecom/` を新規作成

```
.panecom/
├── config.yaml          # プロファイル設定
├── sessions/            # ランタイム状態（自動生成）
│   └── <session-hash>/
│       ├── panes/
│       ├── namespaces/
│       └── .session
└── exec/                # exec の一時ファイル（自動削除）
```

`sessions/` と `exec/` はランタイムデータです。`.gitignore` に追加してください:

```
.panecom/sessions/
.panecom/exec/
```

exec のタイムアウト時は `.panecom/exec/<id>/` が残る場合があります。手動で削除して問題ありません。

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
