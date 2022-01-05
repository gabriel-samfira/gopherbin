package main

import (
	"encoding/json"
	"fmt"
	"gopherbin/config"
	"gopherbin/util"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func getSnapConfig() (config.Config, error) {
	var cfg config.Config

	_, err := ensureSecret()
	if err != nil {
		return cfg, err
	}

	data, err := getConfigSection("config")
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func getConfigSection(section string) ([]byte, error) {
	cmd := exec.Command("snapctl", "get", section)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	data := string(stdoutStderr)
	data = strings.Trim(data, "\r\n")
	data = strings.Trim(data, "\n")

	return []byte(data), nil
}

func setConfigSection(section, data string) error {
	cmd := exec.Command("snapctl", "set", fmt.Sprintf("%s=%s", section, data))
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func ensureSecret() (string, error) {
	var secret []byte
	secret, err := getConfigSection("config.apiserver.jwt-auth.secret")
	if err != nil {
		return "", err
	}

	ret := string(secret)
	if len(secret) == 0 {
		ret, err = util.GetRandomString(32)
		if err != nil {
			return "", nil
		}

		if err := setConfigSection("apiserver.jwt-auth.secret", ret); err != nil {
			return "", err
		}

	}

	return ret, nil
}

func main() {
	configDir := os.Getenv("SNAP_COMMON")
	if configDir == "" {
		log.Fatal("failed to get SNAP_COMMON environment variable")
	}

	cfgFile := filepath.Join(configDir, "gopherbin.toml")
	cfg, err := getSnapConfig()
	if err != nil {
		log.Fatal(err)
	}

	cfgHandle, err := os.Create(cfgFile)
	if err != nil {
		log.Fatal(err)
	}

	encoder := toml.NewEncoder(cfgHandle)
	if err := encoder.Encode(cfg); err != nil {
		log.Fatal(err)
	}
}
