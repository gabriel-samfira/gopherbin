// Copyright 2026 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gopherbin/params"
)

type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *apiClient) login(username, password string) (string, error) {
	body, _ := json.Marshal(params.PasswordLoginParams{
		Username: username,
		Password: password,
	})
	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed: %s: %s", resp.Status, string(raw))
	}
	var jwt params.JWTResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwt); err != nil {
		return "", err
	}
	if jwt.Token == "" {
		return "", fmt.Errorf("login returned empty token")
	}
	c.token = jwt.Token
	return jwt.Token, nil
}

func (c *apiClient) createPaste(p params.Paste) (*params.Paste, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/paste", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create paste failed: %s: %s", resp.Status, string(raw))
	}
	var out params.Paste
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

var errUnauthorized = fmt.Errorf("unauthorized")
