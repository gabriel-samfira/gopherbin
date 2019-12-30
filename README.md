# Gopherbin

Welcome to Gopherbin. This project offers a simple password protected, paste-like service, that you can self host. This is an initial release, so expect bugs.

## Building

You will need at least Go version ```1.13```.

Gopherbin uses [packr2](https://github.com/gobuffalo/packr/tree/master/v2) to bundle static files within the final binary. One of the beauties of Go is the fact that you get a single binary that you can simply distribite and run, without any dependencies. This should be true even with web applications that need to serve up static files.

Install ```packr2```:

```bash
go get -u github.com/gobuffalo/packr/v2/packr2
```

Clone Gopherbin:

```bash
git clone https://github.com/gabriel-samfira/gopherbin
```

Generate packr2 boxes:

```bash
cd gopherbin/templates
packr2
```

Build the binary:

```bash
# Build for GNU/Linux
GOOS=linux go build -o /tmp/gopherbin -mod vendor ../cmd/gopherbin/gopherbin.go

# Or if you prefer Windows
GOOS=windows go build -o /tmp/gopherbin.exe -mod vendor ../cmd/gopherbin/gopherbin.go

# Or to build for a Mac
GOOS=darwin go build -o /tmp/gopherbin -mod vendor ../cmd/gopherbin/gopherbin.go

```

## Creating a database

Gopherbin uses a MySQL/MariaDB database. Install either one of these two, using your prefered method/howto, then create a database that Gopherbin can use:

```sql
create database pastedb;
grant all on pastedb.* to 'pasteuser'@'%' identified by 'superSecretPassword';
flush privileges;
```

## Configuration

The config is a simple toml.


```toml
[default]
registration_open = false
allow_anonymous = false

[apiserver]
bind = "0.0.0.0"
port = 9997
use_tls = false
# Use a decently secure secret. Obviously this needs to be changed :-).
session_secret = "beerdesOwshitvobkeshyijuchepavbiejCefJubemrirjOnJeutyucHalHushbo"

#    [apiserver.tls]
#    certificate = "/path/to/cert.pem"
#    key = "/path/to/key.pem"
#    ca_certificate = "/path/to/ca_cert.pem"

[database]
backend = "mysql"

    [database.mysql]
    username = "pasteuser"
    # This obviously also needs to be changed :-)
    password = "superSecretPassword"
    hostname = "192.168.100.10"
    database = "pastedb"
```

## First run

Before you can use Gopherbin, you need to create the super user. This user is the admin of the system, which can create new users and regular admins. Gopherbin will not allow anyone to log in if this user is missing. The super user can create regular administrators, that can in turn create regular users.

Anyway, let's get to it:

```bash
/tmp/gopherbin create-superuser \
    -config /tmp/config.toml \
    -email example@example.com \
    -fullName "John Doe" \
    -password SuperSecretPassword
```

Then you can simply run the Gopherbin service:

```bash
/tmp/gopherbin run -config /tmp/config.toml
```

If you're running on your local machine, you should be able to access Gopherbin at:

```bash
http://127.0.0.1:9997
```

Otherwise, use your own server's IP address.