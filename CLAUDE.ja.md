# panecom 開発ガイド

**日本語** | [English](CLAUDE.md)

## ビルドとテスト

```bash
go build -o bin/panecom .
go install .
go vet ./...
go test ./... -race
gofmt -w main.go main_test.go
```

## panecom の状態ディレクトリ

`.panecom/` 配下の構造:

- `sessions/` — role ↔ pane のマッピング（**消さない**）
- プロファイル設定はプロジェクトルートの `.panecom.yaml`（旧 `.panecom/config.yaml` も読み取り可能）

v3.0.0 で `exec` コマンドを削除したため、`exec/` と `exec.lock/` は作られなくなった。古い環境に残っていれば消してよい。

### state を消すときの注意

`.panecom/sessions/` を消すと**全 role の登録が失われる**。消さないこと。stale な登録を整理したい場合は `panecom cleanup` を使う。

## skill の変更

`.claude/skills/panecom.md` を変更したら、`AGENTS.md` を再生成する:

```bash
go run . skill sync AGENTS.md
```

`AGENTS.md` は skill のインストール済みコピーで、自動では追従しない。CI の `skill-sync` ジョブが `skill sync --check AGENTS.md` を実行するため、忘れると fail する。

skill は英語で書くが、日本語ユーザーの体験は変えない。トリガーとなる指示例は日英の両方を載せ、「ユーザーが話している言語で返す」ことを skill 自身がエージェントに指示する。編集するときはこの 2 点を壊さないこと。

## ドキュメント

英語版が正で、日本語版はそのミラー。

| 英語 | 日本語 |
|---|---|
| `README.md` | `README.ja.md` |
| `CLAUDE.md` | `CLAUDE.ja.md` |

片方を変更したら、同じ PR でもう片方も更新する。自動チェックは無い。

## ブランチ運用

- main に直接 push しない
- ブランチで作業して PR 経由でマージする
- release-please が自動で release PR を作成する

## macOS / Linux 混在環境

コンテナ (Linux) と macOS で同じ Zellij セッションを共有する場合:
- `go install .` で PATH にバイナリを入れる（両方の OS で）
- `panecom` コマンドは PATH にある前提で動作する
