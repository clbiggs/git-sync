# Go Repository Template

[![Keep a Changelog](https://img.shields.io/badge/changelog-Keep%20a%20Changelog-%23E05735)](CHANGELOG.md)
[![Go Reference](https://pkg.go.dev/badge/github.com/clbiggs/git-sync.svg)](https://pkg.go.dev/github.com/clbiggs/git-sync)
[![go.mod](https://img.shields.io/github/go-mod/go-version/clbiggs/git-sync)](go.mod)
[![LICENSE](https://img.shields.io/github/license/clbiggs/git-sync)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/clbiggs/git-sync)](https://goreportcard.com/report/github.com/clbiggs/git-sync)
[![Codecov](https://codecov.io/gh/clbiggs/git-sync/branch/main/graph/badge.svg)](https://codecov.io/gh/clbiggs/git-sync)

⭐ `Star` this repository if you find it valuable and worth maintaining.

👁 `Watch` this repository to get notified about new releases, issues, etc.

## Description

This is an application with the intention of making sure that a git repository is up to date with its origin.

This is done by the following:
- Polling of the origin repository.
- Immediate Pull and reset by api endpoint.

If the local repository does not exist it will be cloned.

The repository is only updated during polling if the latest commit has a different hash than the last pulled, or upon
application startup.

## Build

### Terminal

- `make` - execute the build pipeline.
- `make help` - print help for the [Make targets](Makefile).

## Release

The release workflow is triggered each time a tag with `v` prefix is pushed.

_CAUTION_: Make sure to understand the consequences before you bump the major version.
More info: [Go Wiki](https://github.com/golang/go/wiki/Modules#releasing-modules-v2-or-higher),
[Go Blog](https://blog.golang.org/v2-go-modules).

## Maintenance

Notable files:

- [.github/workflows](.github/workflows) - GitHub Actions workflows,
- [.github/dependabot.yml](.github/dependabot.yml) - Dependabot configuration,
- [.vscode](.vscode) - Visual Studio Code configuration files,
- [.golangci.yml](.golangci.yml) - golangci-lint configuration,
- [.goreleaser.yml](.goreleaser.yml) - GoReleaser configuration,
- [Dockerfile](Dockerfile) - Dockerfile used by GoReleaser to create a container image,
- [Makefile](Makefile) - Make targets used for development, [CI build](.github/workflows) and [.vscode/tasks.json](.vscode/tasks.json),

## FAQ

### How can I build on Windows

Install [tdm-gcc](https://jmeubank.github.io/tdm-gcc/)
and copy `C:\TDM-GCC-64\bin\mingw32-make.exe`
to `C:\TDM-GCC-64\bin\make.exe`.
Alternatively, you may install [mingw-w64](http://mingw-w64.org/doku.php)
and copy `mingw32-make.exe` accordingly.

Take a look [here](https://github.com/docker-archive/toolbox/issues/673#issuecomment-355275054),
if you have problems using Docker in Git Bash.

You can also use [WSL (Windows Subsystem for Linux)](https://docs.microsoft.com/en-us/windows/wsl/install-win10)
or develop inside a [Remote Container](https://code.visualstudio.com/docs/remote/containers).
However, take into consideration that then you are not going to use "bare-metal" Windows.

Consider using [goyek](https://github.com/goyek/goyek)
for creating cross-platform build pipelines in Go.

### How can I customize the release

Take a look at GoReleaser [docs](https://goreleaser.com/customization/)
as well as [its repo](https://github.com/goreleaser/goreleaser/)
how it is dogfooding its functionality.
You can use it to add deb/rpm/snap packages, Homebrew Tap, Scoop App Manifest etc.

If you are developing a library and you like handcrafted changelog and release notes,
you are free to remove any usage of GoReleaser.

## Contributing

Feel free to create an issue or propose a pull request.

Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
