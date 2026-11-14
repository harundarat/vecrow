# syntax=docker/dockerfile:1

# One image holds both binaries: /app/api and /app/worker.
#   docker build --build-arg VERSION=$(git describe --always) -t vecrow-backend .

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
ARG COMMIT=none
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w \
        -X github.com/harundarat/vecrow/backend/internal/buildinfo.Version=${VERSION} \
        -X github.com/harundarat/vecrow/backend/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /out/worker /app/
USER nonroot:nonroot
EXPOSE 8080
CMD ["/app/api"]
