# Local development

This guide covers setting up a development machine and the day-to-day commands. Everything here runs locally; no Google Cloud account is needed until the cloud phases.

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.27+ | Build and test |
| [Docker Desktop](https://www.docker.com/products/docker-desktop/) | Recent, with Compose v2 | Container images and the local stack |
| GNU make | 4.x | Task runner |
| [golangci-lint](https://golangci-lint.run/) | v2 | Linting and formatting |
| git | 2.x | Version control |

### Windows

The project is developed on native Windows as well as Linux; the Makefile works under both `cmd.exe` and POSIX shells.

```powershell
winget install GoLang.Go
winget install Docker.DockerDesktop
winget install ezwinports.make
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

Restart the terminal afterwards so the updated `PATH` is picked up. `go install` places `golangci-lint.exe` in `%USERPROFILE%\go\bin`, which must be on your `PATH`.

Line endings are normalised to LF by `.gitattributes`, so files behave the same inside Linux containers regardless of your editor.

### macOS / Linux

Install Go and Docker from their websites or your package manager. `make` is usually already installed. Install golangci-lint with the same `go install` command as above.

## Getting started

```sh
git clone https://github.com/itzikyis/docflow-ai.git
cd docflow-ai
make check      # vet, lint and tests
make build      # binaries in bin/
make run        # run the API locally
```

## Make targets

Run `make` with no arguments to list them.

| Target | What it does |
|---|---|
| `make build` | Build every service binary into `bin/` with the version from `git describe` |
| `make run` | Run the API with `go run` |
| `make test` | Run unit tests |
| `make cover` | Run tests and print per-function coverage |
| `make fmt` | Format code (gofmt + goimports) |
| `make vet` | Run `go vet` |
| `make lint` | Run golangci-lint |
| `make check` | `vet`, `lint` and `test`: run this before pushing |
| `make tidy` | Tidy `go.mod` / `go.sum` |
| `make clean` | Remove `bin/` |
| `make docker` | Build a container image per service, tagged with the version and `local` |
| `make up` / `make down` | Start or stop the local stack with Docker Compose |

## Containers

A single multi-stage `Dockerfile` builds every service. The `SERVICE` build argument selects the binary under `cmd/`:

```sh
docker build --build-arg SERVICE=api -t docflow-ai/api:local .
docker run --rm docflow-ai/api:local
```

The runtime image is `distroless/static`: it contains only the static binary and CA certificates, has no shell or package manager, and runs as a non-root user.

## Local stack

`docker-compose.yml` defines the local environment. It grows with the project: PostgreSQL arrives in Phase 2 and the Pub/Sub emulator in Phase 4.

```sh
make up     # build and start
make down   # stop and remove containers
```

## Configuration

Services are configured only through environment variables, following [twelve-factor](https://12factor.net/config) conventions. Each variable is documented here as it is introduced in Phase 1 and later.

## Troubleshooting

**`make: command not found` / `'make' is not recognized`**
Install GNU make (see above) and restart the terminal.

**`failed to connect to the docker API ... dockerDesktopLinuxEngine`**
Docker Desktop isn't running. Start it and wait until the engine reports it's running.

**`golangci-lint: ... Go language version ... is lower than the targeted Go version`**
The linter binary was built with an older Go. Reinstall it with `go install` (above), which builds it with your current Go toolchain.
