// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"gopherbin/apiserver"
	"gopherbin/config"
	"gopherbin/workers/maintenance"

	_ "github.com/go-sql-driver/mysql"
	"github.com/juju/loggo"
)

var log = loggo.GetLogger("gopherbin.cmd")


func getConfig(cfgFile string) (*config.Config, error) {
	cfg, err := config.NewConfig(cfgFile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func runAPIServer(cfgFile string) {
	stop := make(chan os.Signal)
	signal.Notify(stop, syscall.SIGTERM)
	signal.Notify(stop, syscall.SIGINT)
	log.SetLogLevel(loggo.DEBUG)

	cfg, err := getConfig(cfgFile)
	if err != nil {
		log.Errorf("error validating config: %+v", err)
		os.Exit(1)
	}
	apiServer, err := apiserver.GetAPIServer(cfg)
	if err != nil {
		log.Errorf("error getting apiserver: %+v", err)
		os.Exit(1)
	}
	if err := apiServer.Start(); err != nil {
		log.Errorf("error starting api worker: %+v", err)
		os.Exit(1)
	}
	maintenanceWrk, err := maintenance.NewMaintenanceWorker(cfg.Database, cfg.Default)
	if err != nil {
		log.Errorf("error getting maintenance worker: %+v", err)
		os.Exit(1)
	}

	if err := maintenanceWrk.Start(); err != nil {
		log.Errorf("error starting maintenance worker: %+v", err)
		os.Exit(1)
	}
	select {
	case <-stop:
		log.Infof("shutting down gracefully")
		apiServer.Stop()
		maintenanceWrk.Stop()
	}
}

func main() {
	cfgFile := flag.String("config", "", "gopherbin config file")

	flag.Parse()
	if *cfgFile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	runAPIServer(*cfgFile)
}
