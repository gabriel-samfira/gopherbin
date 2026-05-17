// Copyright 2026 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0

// gbcli is a small client for a gopherbin server. It reads stdin and
// creates a paste via the REST API. Pastes are private by default.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopherbin/params"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: gbcli [--config <path>] <command> [flags]

Commands:
  login        Prompt for server URL, username, password and write ~/.gopherbin.toml
  paste        Read stdin and create a paste; prints the paste URL on success

Run 'gbcli <command> -h' for command-specific flags.
`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	configPath := ""
	// Allow --config before the subcommand.
	for len(args) > 0 && (args[0] == "--config" || args[0] == "-config") {
		if len(args) < 2 {
			return errors.New("--config requires a path")
		}
		configPath = args[1]
		args = args[2:]
	}
	if len(args) == 0 {
		usage()
		return errors.New("no command given")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "login":
		return cmdLogin(configPath, rest)
	case "paste":
		return cmdPaste(configPath, rest)
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func cmdLogin(configPath string, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("url", "", "server URL (e.g. https://gopherbin.example.com)")
	username := fs.String("username", "", "username (prompted if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	in := bufio.NewReader(os.Stdin)
	if *server == "" {
		def := cfg.URL
		fmt.Fprintf(os.Stderr, "Server URL [%s]: ", def)
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			line = def
		}
		*server = line
	}
	if *server == "" {
		return errors.New("server URL is required")
	}
	if _, err := url.Parse(*server); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	if *username == "" {
		fmt.Fprint(os.Stderr, "Username: ")
		line, _ := in.ReadString('\n')
		*username = strings.TrimSpace(line)
	}
	if *username == "" {
		return errors.New("username is required")
	}

	password := os.Getenv("GOPHERBIN_PASSWORD")
	if password == "" {
		fmt.Fprint(os.Stderr, "Password: ")
		pw, err := readPassword(in)
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = pw
	}
	if password == "" {
		return errors.New("password is required")
	}

	cfg.URL = strings.TrimRight(*server, "/")
	cfg.Username = *username
	// Do not persist password by default — store the token instead.
	cfg.Password = ""

	client := newClient(cfg.URL, "")
	token, err := client.login(*username, password)
	if err != nil {
		return err
	}
	cfg.Token = token
	if err := cfg.save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Logged in. Token cached in %s\n", cfg.path)
	return nil
}

func cmdPaste(configPath string, args []string) error {
	fs := flag.NewFlagSet("paste", flag.ContinueOnError)
	name := fs.String("name", "", "paste title (default: stdin-<timestamp>)")
	fs.StringVar(name, "n", "", "paste title (shorthand)")
	public := fs.Bool("public", false, "make the paste public (default: private)")
	expires := fs.String("expires", "", "expiration duration, e.g. 1h, 24h, 7d")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if cfg.URL == "" {
		return errors.New("server URL not configured; run 'gbcli login' first")
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	if len(data) == 0 {
		return errors.New("stdin is empty")
	}

	title := *name
	if title == "" {
		title = "stdin-" + time.Now().UTC().Format(time.RFC3339)
	}

	var expPtr *time.Time
	if *expires != "" {
		d, err := parseExpires(*expires)
		if err != nil {
			return err
		}
		t := time.Now().UTC().Add(d)
		expPtr = &t
	}

	payload := params.Paste{
		Data:    data,
		Name:    title,
		Public:  *public,
		Expires: expPtr,
	}

	client := newClient(cfg.URL, cfg.Token)
	out, err := client.createPaste(payload)
	if err == errUnauthorized {
		// Try to refresh the token using stored credentials, if any.
		if cfg.Username == "" || cfg.Password == "" {
			return fmt.Errorf("unauthorized; run 'gbcli login' to refresh the token")
		}
		token, lerr := client.login(cfg.Username, cfg.Password)
		if lerr != nil {
			return lerr
		}
		cfg.Token = token
		_ = cfg.save()
		client = newClient(cfg.URL, token)
		out, err = client.createPaste(payload)
	}
	if err != nil {
		return err
	}

	path := "/p/" + out.PasteID
	if out.Public {
		path = "/public/p/" + out.PasteID
	}
	fmt.Println(cfg.URL + path)
	return nil
}

// readPassword reads a line from stdin with terminal echo disabled via stty.
// If stty is unavailable (e.g. non-tty stdin), it falls back to a visible read
// after warning the user on stderr.
func readPassword(in *bufio.Reader) (string, error) {
	restore := func() {}
	if err := exec.Command("stty", "-F", "/dev/tty", "-echo").Run(); err == nil {
		restore = func() {
			_ = exec.Command("stty", "-F", "/dev/tty", "echo").Run()
			fmt.Fprintln(os.Stderr)
		}
	} else {
		fmt.Fprintln(os.Stderr, "(warning: could not disable echo; password will be visible)")
	}
	defer restore()
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// parseExpires extends time.ParseDuration with a "d" (days) suffix.
func parseExpires(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err != nil {
			return 0, fmt.Errorf("invalid expires: %s", s)
		}
		if days <= 0 {
			return 0, fmt.Errorf("expires must be positive")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
