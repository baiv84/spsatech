FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /helpdesk ./cmd/helpdesk

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 helpdesk \
    && mkdir -p /data/uploads && chown -R helpdesk /data
COPY --from=build /helpdesk /usr/local/bin/helpdesk
USER helpdesk
ENV LISTEN_ADDR=:8091 UPLOAD_DIR=/data/uploads
EXPOSE 8091
ENTRYPOINT ["helpdesk"]
CMD ["serve"]
