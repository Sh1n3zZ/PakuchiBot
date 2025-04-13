FROM golang:1.23-alpine AS builder

WORKDIR /PakuchiBot
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o PakuchiBot .

FROM alpine:latest
WORKDIR /PakuchiBot
COPY --from=builder /PakuchiBot/PakuchiBot .
COPY --from=builder /PakuchiBot/config.example.yaml /config.example.yaml

RUN apk add --no-cache tzdata

CMD ["./PakuchiBot"]
