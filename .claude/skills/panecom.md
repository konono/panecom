---
name: panecom
description: ユーザーから developer / reviewer 等の役割を割り当てられたとき、または Zellij / tmux 上の他 agent との協業を指示されたときに panecom で登録・通信する
---

# panecom

ターミナルマルチプレクサ（Zellij / tmux）の pane 間通信ツール。role 名で相手を指定する。pane ID を直接扱わない。

## 重要: 利用条件

**panecom は自発的に使わない。** 以下のようなユーザーからの明示的な指示があった場合のみ利用する:

- 「共有ターミナルを作って」「terminal 開いて」「新しい pane を作って」
- 「一緒にターミナルで作業しよう」
- 「panecom で〜して」
- 「reviewer に送って」「developer に共有して」
- 「あなたがレビュワーね」「developer として進めて」など、自分への役割指定
- 「別プロジェクトの reviewer と相談して」など、panecom の相手を使う共同作業の指示

**通常のコマンド実行は自分の環境（Bash ツール等）で行う。** panecom 経由だとサンドボックスや権限管理の外で動作する。

## developer / reviewer として参加する

ユーザーが自分を「レビュワー」「reviewer」と指定したら、自分の pane で `panecom register reviewer` を実行する。「開発者」「developer」なら `panecom register developer` を実行する。成功したら `panecom whoami` で role と namespace を確認する。失敗した場合は理由をユーザーに伝え、登録済みとは扱わない。ユーザーが pane を選んで登録する操作は `panecom register`（引数なし）で行う。

同じプロジェクトで `developer` と `reviewer` が協業するときは、次を基本にする:

- reviewer は developer と方針を相談し、実装の依頼を `panecom send developer` で送る。developer が判断に迷ったら `panecom send reviewer` で相談する。
- developer は実装後、commit 前に reviewer にレビューを依頼する。reviewer は差分だけでなくテストと実際の動作も確認し、指摘を返す。
- reviewer はレビュー結果を踏まえて commit、push、PR の指示を出し、developer が実行する。ユーザーや上位の指示に確認・承認条件があればそれを守る。
- reviewer は作業中、おおむね 1 時間ごとに `panecom dump developer` で進捗を確認する。止まっていると判断したら、状況に応じて質問や次の作業を送る。この案内は AI が停止中に自動起動するものではない。

相手が別プロジェクトにいる共同作業を依頼された場合、指定されたリポジトリのパスと alias から `panecom link <alias> <path>` を自分で設定し、`<alias>/<role>` で通信する。相手がまだ未登録で、後から参加することが分かっている場合だけ `panecom link -f` を使う。終了時に一時的なリンクを解除するなら、自分がその作業のために作ったものだけ `panecom unlink <alias>` で解除する。既存リンクや profile が管理するリンクは勝手に解除しない。対象のプロジェクトを特定できないときはパスを確認する。

## マルチプレクサの選択

panecom は Zellij と tmux を自動検出する。明示指定も可能:

```
panecom --mux tmux <command>       # --mux フラグ
PANECOM_MUX=tmux panecom <command> # 環境変数
```

優先順位: `--mux` > `PANECOM_MUX` > 自動検出（`ZELLIJ_SESSION_NAME` > `TMUX`）

## コマンドの使い分け

### 自分のシェルで実行すべきもの（panecom 不要）

- ファイル操作、ビルド、テスト、git 操作など通常の開発作業
- 自分で結果を確認して次の判断をする作業

### panecom を使うもの（ユーザー指示があった場合のみ）

| やりたいこと | コマンド |
|---|---|
| 他の AI エージェントにメッセージを送る | `panecom send <role> "<message>"` |
| 他の AI エージェントの画面を見る | `panecom dump <role>` |
| 共有ターミナルを開く | `panecom open terminal` |
| 共有ターミナルでコマンドを実行する | `panecom exec terminal "<command>"` |
| 自分の画面を相手に見せる（人が使う） | `panecom share <role>` |

## send の配信確認（重要）

`send` は相手の画面に文字を打ち込んで Enter を送るだけなので、相手が**入力モードでない**と黙って捨てられる。
そのため panecom は送信後に相手の画面を自動で確認し（デフォルト ON）、確認できなければ非ゼロで終了する。

| exit code | 意味 | AI がとるべき対応 |
|---|---|---|
| 0 | 送信後に画面が変化し、入力欄にメッセージが残っていない = 受理されたとみなせる | そのまま次へ |
| 1 | role 未解決、pane 消失などのエラー | 設定・role を確認する。再送しない |
| 3 | 画面が全く変化しなかった = **未受理の疑い**。相手が詳細表示・ページャ・ダイアログ等で入力を受け付けていない | stderr に出る画面を読んで状態を判断する（下記） |
| 4 | 入力欄にメッセージが残っている = Enter が効いていない | 相手の状態を確認し、必要なら再送する |

非ゼロ終了時は相手画面の末尾 20 行が stderr に出力されるので、**まずそれを読んで状況を判断する**。

### exit 3 / 4 のときの判断

stderr の画面内容から相手の状態を推定する。典型例:

| 画面に見えるもの | 状態 | 対応 |
|---|---|---|
| `Showing detailed transcript` など | Claude が詳細表示中 | ユーザーに「Ctrl+O で入力画面に戻してほしい」と伝える |
| `Do you want to...` / `1. Yes` など | 承認ダイアログ待ち | **勝手にキーを送らない**。ユーザーに判断を仰ぐ |
| `(END)` / `:` | ページャ表示中 | ユーザーに `q` での終了を依頼する |
| `-- INSERT --` 等 | エディタ起動中 | ユーザーに通知する |
| 通常のプロンプトに見える | 相手が処理中で描画が止まっていた可能性 | 少し待って `panecom dump <role>` で再確認。依頼が届いていなければ 1 回だけ再送 |

**やってはいけないこと**: 状態が判断できないまま Enter や Esc、Ctrl+C を送って復帰させようとすること。承認ダイアログに誤入力すると意図しない操作を許可してしまう。

### verify を切る場合

意図的に確認をスキップしたいときだけ使う:

```
panecom send --no-verify <role> "<message>"
panecom send --verify-timeout 10 <role> "<message>"   # 相手の描画が遅い場合
```

## 共有ターミナル (exec) の使い方

ユーザーと一緒にターミナルで作業する場合に使う。

### 前提

1. まず共有ターミナルを開く:
   ```
   panecom open terminal
   ```
2. その後 exec でコマンドを送る:
   ```
   panecom exec terminal "<command>"
   ```

### exec の動作

- コマンドを共有ターミナルの pane で実行する
- **完了まで自動で待機**し、結果を stdout に返す（dump 不要）
- 失敗時は stderr と非ゼロ exit code を返す
- デフォルト 30 秒タイムアウト（`--timeout 60` で変更可能）
- クォーテーションやエスケープは panecom が自動処理する
- **同時に1つしか実行できない**（実行中に別の exec を開始するとエラー）
- 実行中の進捗は `cat .panecom/exec/current/stdout` で確認可能

### exec の例

```
panecom exec terminal "kubectl get pods -A"
panecom exec terminal "docker ps"
panecom exec terminal "cat /etc/os-release"
panecom exec --timeout 120 terminal "make build"
```

### exec と send の違い

| | exec | send |
|---|---|---|
| 対象 | シェルが動いている pane | AI エージェントの pane |
| 動作 | コマンド実行 → 結果を返す | メッセージ送信（Enter 付き） |
| 待機 | 完了まで待つ | 配信確認のみ待つ（数秒） |
| 結果 | stdout に出力される | なし（exit code で配信可否を判定） |

## その他のコマンド

```
panecom register <role>          # 自分の pane を role として登録
panecom register                 # 同じタブの pane を選んで登録（role 入力では Tab 補完）
panecom whoami                   # 自分の role・pane ID・登録時の namespace を確認
panecom whoami --role            # role のみを表示（スクリプト向け）
panecom resolve <role>           # role → pane ID を確認
panecom dump [--full] <role>     # 対象 pane の画面を取得
panecom profile [-f] <name>     # .panecom.yaml のプロファイルで環境構築
panecom link [-f] <alias> <path> # 別プロジェクトの role を参照可能にする
panecom unlink <alias>          # リンクを削除
panecom skill install           # agent と user/project scope を選んで skill を適用
panecom skill list              # panecom が記録した配布先を表示
panecom skill uninstall         # 記録した配布先を選んで解除
```

## ルール

- **ユーザーの明示的な指示なく panecom を使わない**。自分への role 指定と別プロジェクトの相手との協業依頼も明示的な指示に含む
- **通常のコマンド実行は自分の環境で行う（Bash ツール等）**
- pane ID を推測しない
- マルチプレクサの list-panes コマンドから通信相手を推測しない
- exec の結果は stdout で返るので、別途 dump する必要はない
- send は AI エージェントへ、exec はシェル pane へ使う
- **send が非ゼロで終了したら成功扱いにしない**。stderr の画面を読んで原因を判断し、必要ならユーザーに報告する
- 相手の UI 状態を復帰させるキー（Ctrl+O、Esc、q など）を勝手に送らない
