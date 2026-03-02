FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bot ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache librsvg rsvg-convert ca-certificates fontconfig font-noto \
    && fc-cache -f
COPY --from=builder /app/bot /usr/local/bin/bot
CMD ["bot"]
