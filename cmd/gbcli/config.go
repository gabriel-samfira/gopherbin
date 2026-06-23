// Copyright 2026 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type cliConfig struct {
	URL      string `toml:"url"`
	Username string `toml:"username,omitempty"`
	Password string `toml:"password,omitempty"`
	Token    string `toml:"token,omitempty"`

	path string `toml:"-"`
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gopherbin.toml"), nil
}

func loadConfig(path string) (*cliConfig, error) {
	if path == "" {
		p, err := defaultConfigPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	c := &cliConfig{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if _, err := toml.Decode(string(data), c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	c.path = path
	c.URL = strings.TrimRight(c.URL, "/")
	return c, nil
}

func (c *cliConfig) save() error {
	if c.path == "" {
		p, err := defaultConfigPath()
		if err != nil {
			return err
		}
		c.path = p
	}
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
