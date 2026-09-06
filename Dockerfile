# Stage 1: Build binary
FROM golang:alpine AS builder

RUN apk add --no-cache ca-certificates tzdata git bash tar zip

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build arguments for version injection
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# Pre-compile embedded client binaries
RUN ./hack/embed-binaries.sh

# Statically link and strip binary
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X github.com/gosuda/maek/internal/version.Version=${VERSION} \
      -X github.com/gosuda/maek/internal/version.Commit=${COMMIT} \
      -X github.com/gosuda/maek/internal/version.Date=${DATE}" \
    -o /bin/maek ./cmd/maek

# Stage 2: Minimal runtime image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S maek && adduser -S -G maek -u 10001 maek

COPY --from=builder /bin/maek /usr/local/bin/maek

USER maek

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/maek"]
CMD ["server", "-p", "8080"]
