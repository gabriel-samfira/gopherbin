FROM golang:1.16-rc AS builder
LABEL stage=builder

RUN curl -sL https://deb.nodesource.com/setup_18.x > /tmp/setup_node.sh
RUN /bin/bash /tmp/setup_node.sh
RUN apt-get update && apt-get -y install git make nodejs apt-utils
RUN npm install --global yarn

ADD . /build/gopherbin
WORKDIR /build/gopherbin

RUN mkdir /tmp/go
ENV GOPATH /tmp/go

# build gopher binary
RUN make all-ui

# creating a minimal image
FROM scratch

# Copy our binary to the image
COPY --from=builder /tmp/go/bin/gopherbin /gopherbin

# Run binary and expose port
ENTRYPOINT ["/gopherbin", "-config", "/etc/gopherbin-config.toml"]

EXPOSE 9997/tcp
