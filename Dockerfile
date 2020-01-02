FROM golang:alpine AS builder
LABEL stage=builder

RUN apk update && apk add --no-cache git file

RUN go get -u github.com/gobuffalo/packr/v2/packr2 && \
  git clone https://github.com/gabriel-samfira/gopherbin

WORKDIR /go/gopherbin/templates

# build gopher binary
RUN packr2
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -a -ldflags="-w -s" -installsuffix cgo \
  -o /tmp/gopherbin -mod vendor ../cmd/gopherbin/gopherbin.go

# creating a minimal image
FROM scratch

# Copy our binary to the image
COPY --from=builder /tmp/gopherbin /gopherbin

# Run binary and expose port
ENTRYPOINT ["/gopherbin","run", "-config", "/etc/gopherbin-config.toml"]

EXPOSE 9997/tcp
