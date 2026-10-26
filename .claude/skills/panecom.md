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

- エージェント同士のやり取りはすべて `panecom tell` を使う（`send` ではない。下記「tell と send の使い分け」）。
- reviewer は developer と方針を相談し、実装の依頼を `panecom tell developer` で送る。developer が判断に迷ったら `panecom tell reviewer` で相談する。
- developer は実装後、commit 前に reviewer にレビューを依頼する。reviewer は差分だけでなくテストと実際の動作も確認し、**レビュー結果を `panecom tell developer` で返す**。自分の pane に書いて終わりにしない（下記「依頼と応答」）。
- reviewer はレビュー結果を踏まえて commit、push、PR の指示を出し、developer が実行する。ユーザーや上位の指示に確認・承認条件があればそれを守る。
- reviewer は作業中、おおむね 1 時間ごとに `panecom dump developer` で進捗を確認する。止まっていると判断したら、状況に応じて質問や次の作業を送る。この案内は AI が停止中に自動起動するものではない。

相手が別プロジェクトにいる共同作業を依頼された場合、指定されたリポジトリのパスと alias から `panecom link <alias> <path>` を自分で設定し、`<alias>/<role>` で通信する。相手がまだ未登録で、後から参加することが分かっている場合だけ `panecom link -f` を使う。終了時に一時的なリンクを解除するなら、自分がその作業のために作ったものだけ `panecom unlink <alias>` で解除する。既存リンクや profile が管理するリンクは勝手に解除しない。対象のプロジェクトを特定できないときはパスを確認する。

## 依頼と応答（往復を切らさない）

**panecom 経由で受けた依頼は、panecom 経由で返す。** 相手は別の pane にいて、こちらの画面を見ていない。自分の pane に結果を書いただけでは、相手は待ち続ける。

- **受けた側**: 作業が終わったら、または結論・判断が出たら `panecom tell <依頼元 role> "<結果>"` を送る。**送信するまでそのタスクは完了していない**
- **返信先の調べ方**: `tell` で届いたメッセージは 1 行目がヘッダになっている。形は 2 種類あり、**`project=` の有無で返し方が変わる**

  | 届いたヘッダ | 送信元 | 返し方 |
  |---|---|---|
  | `[panecom:from=reviewer]` の 1 行だけ | 同じプロジェクト | `panecom tell reviewer "<返信>"` |
  | 2 行目に `[panecom:project=...]` がある | **別プロジェクト** | `panecom link <alias> <project のパス>` してから `panecom tell <alias>/<from の role> "<返信>"` |

  ```
  [panecom:from=d]
  [panecom:project=/path/to/other project]
  レビューをお願いします
  ```

  ↓ 返し方

  ```
  panecom link other "/path/to/other project"
  panecom tell other/d "<返信>"
  ```

  **ヘッダの読み方**: 各値は 1 行に 1 つで、`[panecom:<キー>=` と行末の `]` の間が値。**空白で区切らない** — プロジェクトのパスには空白が含まれることがあるため、行末までを 1 つの値として読む。シェルに渡すときは引用符で囲む

  **role 名だけで返さない**。role は namespace の中でしか意味を持たないため、別プロジェクトから来た `from=d` にそのまま `panecom tell d` と返すと、自分のプロジェクトの `d` を探してしまう。未登録なら失敗し、**同名の別エージェントが居れば誤送信になる**。`[panecom:project=...]` の行があったら必ず link を経由する（alias は自分で決めてよい。既存の link があるならそれを使う）
- **返す内容**: 結論（approve / changes requested など）、その理由、相手が次に取るべき行動。長くなる場合は要点だけを送り、詳細は PR コメントやファイルに置いてその場所を伝える
- **送る側**: `tell` が自分の role を自動で付けるので、本文で名乗る必要はない。`send` で送ると送信者が付かず、相手は返信先を判断できない

  ```
  panecom tell reviewer "PR #20 のレビューをお願いします: <URL>"
  ```

  相手の pane に届く内容:

  ```
  [panecom:from=developer]
  PR #20 のレビューをお願いします: <URL>
  ```

  別プロジェクトの相手に送ると、相手が返信できるよう自分のプロジェクトのパスが 2 行目に付く:

  ```
  [panecom:from=developer]
  [panecom:project=/path/to/myproject]
  PR #20 のレビューをお願いします: <URL>
  ```

- **依頼の入口は問わない**: 相手 role と進めている共同作業については、ユーザーから pane 内で直接指示された場合でも、その作業の結果を相手 role に `panecom tell` で返す。相手はこちらの画面を見ていないという点は、依頼がどこから来たかで変わらない。**対象はその共同作業の結果に限る**。同じ pane でユーザーから頼まれただけの、共同作業と無関係な作業まで送らない
- **返せないときも黙らない**: 判断できない・情報が足りない場合は、「何が足りないか」を送り返す
- **時間がかかるとき**: 着手した旨を 1 通送ってから作業に入る。相手が待ちか再依頼かを判断できる

違反例: reviewer がレビュー結果を自分の pane に出力しただけで終わり、developer は待ち続け、結局ユーザーが手で内容を転記する。そのレビューがユーザーからの直接指示で始まった場合も同じ（developer と進めている共同作業の結果なので返す）。

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
| 他の AI エージェントにメッセージを送る | `panecom tell <role> "<message>"` |
| pane に文字列をそのまま打ち込む（相手が AI とは限らない） | `panecom send <role> "<text>"` |
| 他の AI エージェントの画面を見る | `panecom dump <role>` |
| 共有ターミナルを開く | `panecom open terminal` |
| 共有ターミナルでコマンドを実行する | `panecom send terminal "<command>"` + `panecom dump terminal` で結果確認 |
| 自分の画面を相手に見せる（人が使う） | `panecom share <role>` |

## tell と send の使い分け（重要）

**相手が AI エージェントなら `tell`、pane をそのまま操作したいなら `send`。** 迷ったら `tell` を使う。

| | `tell` | `send` |
|---|---|---|
| 相手 | role を持つ AI エージェント（人を含む） | 任意の pane（シェル、TUI、エディタ等） |
| 送る内容 | 1 行目に `[panecom:from=<自分の role>]`（別プロジェクト宛なら 2 行目に `[panecom:project=<自分のパス>]`） | **与えた文字列をそのまま**打ち込む |
| 自分の登録 | **必須**（未登録ならエラー） | 不要 |
| 用途 | 依頼・相談・報告・レビュー結果 | コマンド入力、キー送信、プロンプトへの応答 |

```
panecom tell reviewer "PR #20 のレビューをお願いします"   # エージェント宛
panecom send terminal "git status"                        # pane にそのまま打ち込む
```

`send` は汎用のプリミティブで、ヘッダを付けない。シェルや TUI を相手にしたときに `[panecom:from=...]` が混ざると壊れるため。エージェント宛に `send` を使うと送信者が付かず、相手は返信先を判断できない（それが `tell` を分けた理由）。

別プロジェクトの相手（`<alias>/<role>` 形式）に送る場合、ヘッダ 2 行目に自分のプロジェクトのパスが入る。role 名は namespace の中でしか通用せず、相手は role 名だけでは返信できないため。自分のプロジェクトを特定できない場合（登録情報が壊れている等）、`tell` は送信せずエラーにする（返信不能なメッセージを送るより止まる）。返信を受ける側の手順は「依頼と応答」の表を参照。

空のメッセージも送れない。本文が無いとヘッダが配信確認の対象になり、どのメッセージでも同じ文字列を見ることになるため。

`tell` は未登録の pane から実行するとエラーになる。名乗るのが目的のコマンドなので、先に `panecom register <role>` する。名乗らずに文字列を送りたいだけなら `send` を使う。

なお `send` は打ち込むだけで**結果は返らない**。シェル pane でコマンドを実行してその結果が必要な場合は、`send` したあと `panecom dump <role>` で画面を読む。

## 配信確認（tell / send 共通、重要）

`tell` / `send` はどちらも相手の画面に文字を打ち込んで Enter を送るだけなので、相手が**入力モードでない**と黙って捨てられる。
そのため panecom は送信後に相手の画面を自動で確認し（デフォルト ON）、確認できなければ非ゼロで終了する。以下は両コマンドに共通する。

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
panecom tell --no-verify <role> "<message>"
panecom tell --verify-timeout 10 <role> "<message>"   # 相手の描画が遅い場合
panecom send --no-verify <role> "<text>"              # send も同じフラグを持つ
```

## 共有ターミナルの使い方

ユーザーと一緒にターミナルで作業する場合に使う。

1. まず共有ターミナルを開く:
   ```
   panecom open terminal
   ```
2. コマンドを打ち込む:
   ```
   panecom send terminal "kubectl get pods -A"
   ```
3. 結果を読む:
   ```
   panecom dump terminal
   ```

**通常のコマンド実行は自分の環境（Bash ツール）で行う。** 共有ターミナルを使うのは、ユーザーに見せる必要がある、ユーザーの環境・認証情報で動かす必要がある、同じシェルの状態（cd / 環境変数）を使い続ける必要がある、のいずれかに当たる場合だけ。

### 結果の読み取り

`send` は打ち込むだけなので、**完了を待つ仕組みは無い**。`dump` した時点の画面が返る。

- コマンドが終わっているかは `dump` の内容（プロンプトが戻っているか）で判断する
- すぐ終わらないコマンドは、少し待ってから `dump` し直す
- 画面に収まらない出力は取りこぼす。全量が必要なら、コマンド側でファイルにリダイレクトしてから読む:
  ```
  panecom send terminal "make build > /tmp/build.log 2>&1; echo done"
  panecom dump terminal          # done が出ていれば完了
  ```
- `dump` はあくまで画面の写しなので、長い行は pane 幅で折り返される

### tell / send / dump の違い

| | tell | send | dump |
|---|---|---|---|
| 対象 | AI エージェントの pane | 任意の pane | 任意の pane |
| 動作 | 送信者名を付けてメッセージ送信 | 文字列をそのまま打ち込む | 画面を読み取る |
| 結果 | なし（exit code で配信可否を判定） | なし（exit code で配信可否を判定） | 画面の内容が stdout に出る |

## その他のコマンド

```
panecom register <role>          # 自分の pane を role として登録
panecom register                 # 同じタブの pane を選んで登録（role 入力では Tab 補完）
panecom whoami                   # 自分の role・pane ID・登録時の namespace を確認
panecom whoami --role            # role のみを表示（スクリプト向け）
panecom tell <role> "<message>"  # エージェントにメッセージ（送信者名が付く）
panecom send <role> "<text>"     # pane に文字列をそのまま打ち込む
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
- **エージェント宛は `tell`、pane をそのまま操作するなら `send`**。エージェント宛に `send` を使わない（送信者が付かず相手が返信先を判断できない）
- `send` の結果が必要なら `dump` で読む。完了を待つ仕組みは無いので、終わったかどうかは画面で判断する
- **panecom で受けた依頼は panecom で返す**。自分の pane に結果を出力して終わりにしない。返信を送るまでそのタスクは完了していない
- **返信先は届いたメッセージの `[panecom:from=<role>]` を見る**。`tell` が自動で付けるので、本文で名乗る必要はない
- **tell / send が非ゼロで終了したら成功扱いにしない**。stderr の画面を読んで原因を判断し、必要ならユーザーに報告する
- 相手の UI 状態を復帰させるキー（Ctrl+O、Esc、q など）を勝手に送らない
