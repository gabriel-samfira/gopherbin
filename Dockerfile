FROM golang:1.26 AS builder
LABEL stage=builder

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
 && apt-get install -y nodejs

ADD . /build/gopherbin
WORKDIR /build/gopherbin

RUN mkdir /tmp/go
ENV GOPATH=/tmp/go

RUN make with-ui

# creating a minimal image
FROM gcr.io/distroless/base-debian12

COPY --from=builder /tmp/go/bin/gopherbin /gopherbin

ENTRYPOINT ["/gopherbin", "-config", "/etc/gopherbin-config.toml"]

EXPOSE 9997/tcp
