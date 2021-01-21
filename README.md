# Gopherbin

Welcome to Gopherbin. This project offers a simple password protected, paste-like service, that you can self host. This is an initial release, so expect bugs.

## Building

### Using Go

You will need at least Go version ```1.16```.

Gopherbin uses the embed feature in Go to optionally bundle the [Web UI](https://github.com/gabriel-samfira/gopherbin-web). You an choose to build without a web UI and simply serve the needed static files using a proper web server. 

Clone Gopherbin:

```bash
git clone https://github.com/gabriel-samfira/gopherbin
```

If you want to build the UI, you will need a recent version of nodejs and yarn. With those dependencies installed, simply run:

```bash
make all-ui
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

Gopherbin uses a MySQL/MariaDB database. Install either one of these two, using your preferred method/how-to, then create a database that Gopherbin can use:

```sql
create database pastedb;
grant all on pastedb.* to 'pasteuser'@'%' identified by 'superSecretPassword';
flush privileges;
```

## Configuration

The config is a simple toml.

```toml
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
