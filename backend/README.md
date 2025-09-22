# Project backend

One Paragraph of project description goes here

## Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes. See deployment for notes on how to deploy the project on a live system.

## MakeFile

Run build make command with tests

```bash
make all
```

Build the application

```bash
make build
```

Run the application

```bash
make run
```

## API Docs (OpenAPI/Swagger)

- Start the backend and open: http://localhost:8080/swagger/index.html
- The spec is generated using swaggo. To regenerate after editing annotations:

```bash
# generate docs into ./docs
go run github.com/swaggo/swag/cmd/swag@v1.16.3 init -g cmd/api/main.go -o docs -d ./,./internal
```

Notes:

- REST endpoints are annotated above handler functions in `internal/api/rest/*`.
- GraphQL (`POST /api/v1/graphql`) and WebSocket (`GET /api/v1/ws/downloads`) are included with lightweight docs.
- Swagger UI is served at `/swagger/index.html` by gin-swagger.
  Create DB container

```bash
make docker-run
```

Shutdown DB Container

```bash
make docker-down
```

DB Integrations Test:

```bash
make itest
```

Live reload the application:

```bash
make watch
```

Run the test suite:

```bash
make test
```

Clean up binary from the last build:

```bash
make clean
```
