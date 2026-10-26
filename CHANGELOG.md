# Changelog

## [2.1.0](https://github.com/konono/panecom/compare/v2.0.1...v2.1.0) (2026-10-26)


### Features

* add tell for messages between registered roles ([7b0d9ac](https://github.com/konono/panecom/commit/7b0d9ac664b57b6e0a5e5caab0bfc4f3b20a427c))
* add tell for messages between registered roles ([2ec4e88](https://github.com/konono/panecom/commit/2ec4e8888193de2d4be7981359457896d027ea2c)), closes [#23](https://github.com/konono/panecom/issues/23)
* hide stale roles, add cleanup and focus commands ([033a1ac](https://github.com/konono/panecom/commit/033a1accb7b59f87e7e50ff78ea16659d63d0e68))
* hide stale roles, add cleanup and focus commands ([9c2b5e2](https://github.com/konono/panecom/commit/9c2b5e23772f3850ba491a984a26fa2d99e09f40))


### Bug Fixes

* make a cross-project tell answerable ([77d62f3](https://github.com/konono/panecom/commit/77d62f37228cb4ff9d547419fba01aeba23fea1b))
* make the reply address readable and refuse when it is missing ([8cc833f](https://github.com/konono/panecom/commit/8cc833fcc041beb3ed281a257c822d1e96a5e08c))
* satisfy errcheck and ineffassign ([2af3f99](https://github.com/konono/panecom/commit/2af3f99af33c9d9b176f8176dd17935943e1b7c9))
* treat an empty pane list as unreadable, not as no panes ([f63940c](https://github.com/konono/panecom/commit/f63940cbabb11dfdc02515a91e13f27059525b1b))

## [2.0.1](https://github.com/konono/panecom/compare/v2.0.0...v2.0.1) (2026-10-26)


### Bug Fixes

* **skill:** match manifest entries across path aliases ([311979d](https://github.com/konono/panecom/commit/311979de450c5e41daab9bd6ca246fc4d9726f10))
* **skill:** match manifest entries across path aliases ([c716d59](https://github.com/konono/panecom/commit/c716d5940389cce83aef1c9439bb360a66264b17))

## [2.0.0](https://github.com/konono/panecom/compare/v1.0.0...v2.0.0) (2026-10-26)


### ⚠ BREAKING CHANGES

* **whoami:** panecom whoami now prints labeled role, pane, and namespace lines by default. Use panecom whoami --role for the previous role-only output.

### Features

* interactive pane registration and namespace diagnostics ([509f26d](https://github.com/konono/panecom/commit/509f26d7083113c070e425546159c6a90210100f))
* **namespace:** warn about live legacy registrations ([e9e3d87](https://github.com/konono/panecom/commit/e9e3d87371491bfc036b64344fdf36f73e2c0fb3))
* portable profiles and guided team setup ([cac20f3](https://github.com/konono/panecom/commit/cac20f33c0d300df2e6845b1c400977668a2df92))
* **profile:** load project config from root .panecom.yaml ([6da101e](https://github.com/konono/panecom/commit/6da101ea8fe9a3e6b4b937812a630cb831e8799d))
* **register:** cancel with Esc and highlight selected pane ([f5bc351](https://github.com/konono/panecom/commit/f5bc3513c7351ebe60d876ec2648287ddc1b90bb))
* **register:** navigate panes by screen position ([a95a932](https://github.com/konono/panecom/commit/a95a93287a9025b38460ec388496d24f1b9083e8))
* **register:** select and name an existing pane interactively ([e75a23c](https://github.com/konono/panecom/commit/e75a23cd4b1fcd4da644c0b8a7503135d61dacac))
* **skill:** add guided install and uninstall ([89789d4](https://github.com/konono/panecom/commit/89789d434fa667bc6cb5304d1d4063c75ee0924d))
* **whoami:** show pane ID and registered namespace ([5963661](https://github.com/konono/panecom/commit/5963661c097ebd84778394e591253bfd8e994ca2))


### Bug Fixes

* **ci:** handle close and warning write results ([c66e0b4](https://github.com/konono/panecom/commit/c66e0b488a634ca91d82ac8e69af4892161da451))
* **deploy:** replace multiline skill sections with awk input file ([4edc0a5](https://github.com/konono/panecom/commit/4edc0a5024381a8741ba77c6668661fe44099e3c))
* **namespace:** refine legacy registration guidance ([c4cd6da](https://github.com/konono/panecom/commit/c4cd6da3ac1177b626b6fa26b3054dffc8349152))
* **register:** address interactive picker review findings ([5a2a53b](https://github.com/konono/panecom/commit/5a2a53baa91d626b387aea742c0839b4879d276f))
* **register:** limit visual picker to current tab ([b3e0c66](https://github.com/konono/panecom/commit/b3e0c66e59331220fb9b36ecba2b00a0d4cd6ccf))
* **register:** read arrow sequences on macOS PTYs ([896bfd8](https://github.com/konono/panecom/commit/896bfd89986ddc95c5837811b5489158ae58e6d5))
* **release:** report the version embedded in release binaries ([5e1727b](https://github.com/konono/panecom/commit/5e1727baa71397fedcdbcc43eb2f5587622d31b6))
* **skill:** check deferred cleanup in lint ([f68fb8e](https://github.com/konono/panecom/commit/f68fb8ea4472e7578fffc28f449d7abb82d4f88c))
* **skill:** deduplicate symlinked install destinations ([30cc5b9](https://github.com/konono/panecom/commit/30cc5b9b2d2b60c48244806280a8f46a468afa1a))
* **skill:** preserve adopted files and permissions ([f72fa55](https://github.com/konono/panecom/commit/f72fa55f9acad2bdb29652af1f2acc7c18e851bf))
* **whoami:** add role-only output and sync guidance ([c389944](https://github.com/konono/panecom/commit/c389944f63aff4ac0d92a36d78b85b2834b719a6))
* **whoami:** show unknown for missing namespace metadata ([005292f](https://github.com/konono/panecom/commit/005292f51eb1f9a32c6d066490e6f20cd3416976))

## [1.0.0](https://github.com/konono/panecom/compare/v0.4.0...v1.0.0) (2026-10-26)


### ⚠ BREAKING CHANGES

* **namespace:** namespace identity changed. Panes registered by an earlier version must run `panecom register` again before they can see panes registered by this one.

### Features

* **link:** address roles in other projects as project/role ([f27d000](https://github.com/konono/panecom/commit/f27d000dc9a95ae4e2503fb3933aac8551d07804))


### Bug Fixes

* anchor namespaces and link roles across projects ([051c42e](https://github.com/konono/panecom/commit/051c42e651055f19ed2eaa8f5c19e2cfaa2ad114))
* **namespace:** anchor namespaces to the project root ([045fc39](https://github.com/konono/panecom/commit/045fc3925d0fe9f48fc805a2aa52a7b2dc0f690e))

## [0.4.0](https://github.com/konono/panecom/compare/v0.3.0...v0.4.0) (2026-10-20)


### Features

* **send:** verify delivery and surface recipient state via exit codes ([43c9e54](https://github.com/konono/panecom/commit/43c9e5449048b6001324cbe7248546f87a4b3ba6))
* **send:** verify delivery and surface recipient state via exit codes ([fc1f11a](https://github.com/konono/panecom/commit/fc1f11abc433b2236fefa89ab4d9aea0af62d7c7))


### Bug Fixes

* **send:** detect unsent input that wraps across screen lines ([44682de](https://github.com/konono/panecom/commit/44682de359e3fbc172dc4c3f4e503320b8f9cae2))
* **send:** treat an empty input line as delivered ([4e8c43f](https://github.com/konono/panecom/commit/4e8c43f6440c041b1dc53c9403e5b4405a28bf80))

## [0.3.0](https://github.com/konono/panecom/compare/v0.2.0...v0.3.0) (2026-09-29)


### Features

* add tmux backend with multiplexer plugin architecture ([#7](https://github.com/konono/panecom/issues/7)) ([1395ee0](https://github.com/konono/panecom/commit/1395ee047b4020aca84f16ba24de738a129b9493))
* add update command and --version flag ([#9](https://github.com/konono/panecom/issues/9)) ([642f469](https://github.com/konono/panecom/commit/642f469ab5808e174fb131690f447767e70d482f))


### Bug Fixes

* use t.Setenv instead of os.Setenv in tests ([662663b](https://github.com/konono/panecom/commit/662663b979c617e4afcf730d51690e2ab7c36249))

## [0.2.0](https://github.com/konono/panecom/compare/v0.1.2...v0.2.0) (2026-09-29)


### Features

* add -l/--lines option to dump and share commands ([#5](https://github.com/konono/panecom/issues/5)) ([bd9bc4b](https://github.com/konono/panecom/commit/bd9bc4b4092f05e1801864bacf8864150cb0dce2))

## [0.1.2](https://github.com/konono/panecom/compare/v0.1.1...v0.1.2) (2026-09-29)


### Bug Fixes

* disable pagers in exec to prevent hanging ([8b26478](https://github.com/konono/panecom/commit/8b264784b77d94e68a782b6d590d723ccab2a6d7))
* prevent interactive commands from hanging exec ([1c7a1e5](https://github.com/konono/panecom/commit/1c7a1e54cc56aab055f9dc189d5eb9b36cee42b4))
* prevent interactive commands from hanging exec ([573e5c8](https://github.com/konono/panecom/commit/573e5c82f3df184f26ad1e31bc5651bb9c35c9c3))
* remove redundant command echo in exec runner ([e84b685](https://github.com/konono/panecom/commit/e84b68501c10e36e078cf1234e56beb193091527))
* show command instead of execID in terminal display ([0baa011](https://github.com/konono/panecom/commit/0baa0111db5f134855e9ddd2838ced4837849926))
* use PATH-based binary name, cleanup orphan exec dirs, improve skill ([#4](https://github.com/konono/panecom/issues/4)) ([7d514ed](https://github.com/konono/panecom/commit/7d514ed5f26cf09ce104ec7f79245651a31c5e35))

## [0.1.1](https://github.com/konono/panecom/compare/v0.1.0...v0.1.1) (2026-09-29)


### Bug Fixes

* handle exec data plane write/close errors ([5db70ad](https://github.com/konono/panecom/commit/5db70ad49c5313ba35fc85a94cdd4b45c8441713))
* propagate exec data-plane errors to exit code ([62f1fb5](https://github.com/konono/panecom/commit/62f1fb5c85a9a71ac600c6a40b2454a1bf567f41))
* resolve all errcheck and staticcheck lint errors ([a5dabad](https://github.com/konono/panecom/commit/a5dabadd3c4cfafea67c3bacf5450b0eec4cdc66))
