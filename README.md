# Gopherbin

Welcome to Gopherbin. This project offers a simple password protected, paste-like service, that you can self host. This is an initial release, so expect bugs.

## Building

### Using Go

You will need at least Go version ```1.16```.

Gopherbin uses the embed feature in Go to optionally bundle the [Web UI](https://github.com/gabriel-samfira/gopherbin-web). You can choose to build without a web UI and simply serve the needed static files using a proper web server. 

Clone Gopherbin:

```bash
git clone https://github.com/gabriel-samfira/gopherbin
```

If you want to build the UI, you will need a recent version of nodejs and yarn. With those dependencies installed, simply run:

```bash
make all
```

Building without a UI:

```bash
make all-noui
```

### Building a docker image

```bash
# For a full list of available variables and commands run: make help

# creating docker image
make build-image

# start a container using image previously built
make start-container

```

## Creating a database

Gopherbin can use either MySQL/MariaDB or SQLite3.

If you're planning on using MySQL, you'll need to create the database first:

```sql
create database gopherbin;
create user 'gopherbin'@'%' identified by 'superSecretPassword';
grant all on gopherbin.* to 'gopherbin'@'%';
flush privileges;
```

## Configuration

The config is a simple toml.

```toml
[apiserver]
bind = "0.0.0.0"
port = 9997
use_tls = false

    [apiserver.jwt_auth]
    # secret used to sign jwt tokens
    #
    secret = "beerdesOwshitvobkeshyijuchepavbiejCefJubemrirjOnJeutyucHalHushbo"
    # the duration, a token will be valid for
    # format is of the form 4m41s
    time_to_live = "1h"

    # [apiserver.tls]
    # certificate = "/path/to/cert.pem"
    # key = "/path/to/key.pem"
    # ca_certificate = "/path/to/ca_cert.pem"

[database]
# Valid options are: mysql, sqlite3
backend = "sqlite3"

    # [database.mysql]
    # username = "gopherbin"
    # # This obviously also needs to be changed :-)
    # password = "superSecretPassword"
    # hostname = "192.168.100.10"
    # database = "gopherbin"

    [database.sqlite3]
    db_file = "/tmp/gopherbin.sql"
```

## First run

Simply run the Gopherbin service. Gopherbin will create the database tables automatically:

```bash
/tmp/gopherbin -config /tmp/config.toml
```

Before you can use Gopherbin, you need to create the super user. This user is the admin of the system, which can create new users and regular admins. Gopherbin will not allow anyone to log in if this user is missing. The super user can create regular administrators, that can in turn create regular users.

Anyway, let's get to it:

```bash
# Make sure you change the password

curl -0 -X POST http://127.0.0.1:9997/api/v1/first-run/ \
	-H "Content-type: application-json" \
	--data-binary @- << EOF
	{
		"email": "example@example.com",
		"full_name": "John Doe",
		"password": "ubdyweercivIch"
	}
EOF
```

If you're running on your local machine, you should be able to access Gopherbin at:

```bash
http://127.0.0.1:9997
```

Otherwise, use your own server's IP address.
