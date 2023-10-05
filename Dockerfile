FROM golang:1.19-alpine as buildbase

WORKDIR /go/src/github.com/rarimo/evm-identity-saver-svc
COPY vendor .
COPY . .

ENV GO111MODULE="on"
ENV CGO_ENABLED=1
ENV GOOS="linux"

RUN apk add build-base
RUN go build -o /usr/local/bin/evm-identity-saver-svc github.com/rarimo/evm-identity-saver-svc

###

FROM alpine:3.9

COPY --from=buildbase /usr/local/bin/evm-identity-saver-svc /usr/local/bin/evm-identity-saver-svc
RUN apk add --no-cache ca-certificates

ENTRYPOINT ["evm-identity-saver-svc"]
