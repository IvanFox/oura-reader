FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /oura-reader ./cmd/oura-reader

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /oura-reader /usr/local/bin/oura-reader
VOLUME /data
ENV OURA_DB_PATH=/data/oura.db
EXPOSE 8080
ENTRYPOINT ["oura-reader"]
CMD ["serve"]
