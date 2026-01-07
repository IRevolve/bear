FROM golang:1.25-alpine AS builder

ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/irevolve/bear/commands.Version=${VERSION}" -o bear .

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/bear /bear

ENTRYPOINT ["/bear"]
