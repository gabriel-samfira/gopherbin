FROM golang:1.26-rc AS builder
LABEL stage=builder

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
  && apt-get install -y nodejs

ADD . /build/gopherbin
WORKDIR /build/gopherbin

RUN mkdir /tmp/go
ENV GOPATH=/tmp/go

# build gopher binary
RUN make all

FROM gcr.io/distroless/base-debian12

# Copy our binary to the image
COPY --from=builder /tmp/go/bin/gopherbin /gopherbin

# Run binary and expose port
ENTRYPOINT ["/gopherbin", "-config", "/etc/gopherbin-config.toml"]

EXPOSE 9997/tcp
