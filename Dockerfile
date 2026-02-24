FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/indexer ./cmd/indexer
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/indexer /usr/local/bin/indexer
COPY --from=builder /bin/api /usr/local/bin/api
COPY migrations /migrations
COPY configs /configs

ENTRYPOINT ["indexer"]
