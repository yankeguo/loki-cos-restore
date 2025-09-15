FROM alpine:3.22

RUN apk add --no-cache ca-certificates

ADD loki-cos-restore /loki-cos-restore