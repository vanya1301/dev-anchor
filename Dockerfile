# syntax=docker/dockerfile:1

# test stage: containerized `go test ./...` — used by CI so tests run in a
# clean, reproducible environment identical for every contributor/runner.
FROM golang:1.26 AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go vet ./... && CGO_ENABLED=0 go test ./...

# build stage: produces the da binary for the container's native platform.
# Not used by CI's release matrix (which cross-compiles on the host), but
# useful for `docker build --target build` local sanity checks.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/da ./cmd/da

FROM scratch AS export
COPY --from=build /out/da /da
