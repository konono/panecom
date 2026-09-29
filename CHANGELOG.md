# Changelog

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
