# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /axon-chief ./cmd/axon-chief

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /axon-chief /app/axon-chief

# Environment variables (to be overridden)
ENV PORT=9100

EXPOSE 9100

ENTRYPOINT ["/app/axon-chief"]
