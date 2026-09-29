<!-- panecom-skill:start -->

# panecom

Zellij pane 間の通信ツール。role 名で相手を指定する。pane ID を直接扱わない。

## 重要: 利用条件

**panecom は自発的に使わない。** 以下のようなユーザーからの明示的な指示があった場合のみ利用する:

- 「共有ターミナルを作って」「terminal 開いて」「新しい pane を作って」
- 「一緒にターミナルで作業しよう」
- 「panecom で〜して」
- 「reviewer に送って」「developer に共有して」

**通常のコマンド実行は自分の環境（Bash ツール等）で行う。** panecom open / exec を使って外部 pane で実行すると、Claude Code のサンドボックスや権限管理の外で動作するため、ユーザーの許可なく使ってはならない。

## コマンド

自分を登録:
```
panecom register <role>
```

自分の role を確認:
```
panecom whoami
```

新しい pane を開いて role を登録（同じ tab 内、フォーカスを奪わない）:
```
panecom open <role>
panecom open -d right <role>
panecom open -d down <role>
```

相手の存在を確認:
```
panecom resolve <role>
```

相手の現在画面を確認:
```
panecom dump <role>
```

過去の出力を含めて確認:
```
panecom dump --full <role>
```

相手にメッセージを送る:
```
panecom send <role> "<message>"
```

相手の pane でコマンドを実行し、結果を受け取る:
```
panecom exec <role> "<command>"
panecom exec --timeout 60 <role> "<command>"
```

自分の画面を相手に共有する（人が使う）:
```
panecom share <role>
panecom share --full <role>
```

## exec の使い方

exec はコマンドをそのまま文字列で渡す。クォーテーションやエスケープは panecom が自動処理する。

```
panecom exec terminal "echo 'hello' && echo world"
panecom exec terminal "ls -la | grep '.go'"
panecom exec terminal "kubectl get pods -A"
```

- コマンド完了まで自動で待機する（デフォルト 30 秒タイムアウト）
- stdout を返す。dump 不要
- 失敗時は stderr と非ゼロ exit code を返す
- 長いコマンドは `--timeout` で秒数を指定する

## 共有ターミナルの運用

ユーザーが「一緒にターミナルで確認しよう」等の指示をした場合のみ:

1. AI が共有ターミナルを作成:
   ```
   panecom open terminal
   ```

2. AI がコマンドを実行して結果を受け取る:
   ```
   panecom exec terminal "kubectl get pods -A"
   ```

3. 人がターミナルで手動操作した後、AI に画面を共有:
   ```
   panecom share developer
   ```

## プロファイルで環境を一発構築

`.panecom/config.yaml` にプロファイルを定義し、一発で開発環境を構築する:
```
panecom profile review
```

config.yaml の例:
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

- 新しい tab を作成し、プロファイル名をタブ名にする
- 各 pane を配置して自動 register
- `cmd` でエージェントやシェルを起動
- `foreground: true` の pane にフォーカス

## ルール

- **ユーザーの明示的な指示なく panecom を使わない**
- **通常のコマンド実行は自分の環境で行う（Bash ツール等）**
- pane ID を推測しない
- `zellij action list-panes` から通信相手を推測しない
- 他 agent の操作には role 名と panecom を使用する
- exec の結果は stdout で返るので、別途 dump する必要はない
<!-- panecom-skill:end -->
