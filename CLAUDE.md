# panecom 開発ガイド

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
- `exec/` — exec のランタイムデータ（テスト後に消してよい）
- `exec.lock/` — exec のロックディレクトリ（テスト後に消してよい）
- `config.yaml` — プロファイル設定（消さない）

### テスト時の state クリア

exec のテスト後にクリアする場合は **exec 関連だけ** を消すこと。sessions を消すと全 role の登録が失われる。

```bash
# 正しい
rm -rf .panecom/exec .panecom/exec.lock*

# やってはいけない
rm -rf .panecom/sessions  # role 登録が全部消える
```

## ブランチ運用

- main に直接 push しない
- ブランチで作業して PR 経由でマージする
- release-please が自動で release PR を作成する

## macOS / Linux 混在環境

コンテナ (Linux) と macOS で同じ Zellij セッションを共有する場合:
- `go install .` で PATH にバイナリを入れる（両方の OS で）
- `panecom` コマンドは PATH にある前提で動作する
