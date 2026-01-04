FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/worker .

FROM scratch
COPY --from=builder /app/worker /worker
ENTRYPOINT ["/worker"]
