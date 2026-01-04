FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY *.go ./

RUN go build -o /hello-api .

FROM alpine:latest

WORKDIR /
COPY --from=builder /hello-api /hello-api

ENV PORT=8080
EXPOSE 8080

CMD ["/hello-api"]
