package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"gopherbin/apiserver"
	"gopherbin/config"

	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/juju/loggo"
)

var log = loggo.GetLogger("coriolis.logger.cmd")

func main() {
	stop := make(chan os.Signal)
	signal.Notify(stop, syscall.SIGTERM)
	signal.Notify(stop, syscall.SIGINT)
	log.SetLogLevel(loggo.DEBUG)

	cfgFile := flag.String("config", "", "gopherbin config file")
	flag.Parse()

	if *cfgFile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	cfg, err := config.NewConfig(*cfgFile)
	if err != nil {
		log.Errorf("error validating config: %q", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		log.Errorf("failed to validate config: %q", err)
		os.Exit(1)
	}
	apiServer, err := apiserver.GetAPIServer(cfg)
	if err != nil {
		log.Errorf("error getting apiserver: %v", err)
		os.Exit(1)
	}
	if err := apiServer.Start(); err != nil {
		log.Errorf("error starting api worker: %q", err)
		os.Exit(1)
	}
	select {
	case <-stop:
		log.Infof("shutting down gracefully")
		apiServer.Stop()
	}
}
