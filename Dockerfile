# Stage 1: Build
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# Cache go mod downloads
COPY go-validator/go.mod go-validator/go.sum ./
RUN go mod download

# Copy source
COPY go-validator/ .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/startup-factory .

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/startup-factory /app/startup-factory
COPY profile.json /app/profile.json

# Data dir for any CLI-mode artifacts
RUN mkdir -p /app/results && chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

ENV PORT=8080

CMD ["/app/startup-factory", "-mode", "server", "-port", "8080"]
