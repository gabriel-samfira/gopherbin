package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gopherbin/admin"
	"gopherbin/apiserver"
	"gopherbin/config"
	"gopherbin/params"

	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/juju/loggo"
)

var log = loggo.GetLogger("gopherbin.cmd")
var usage = `Available subcommands:
%s init
%s run
`

func printUsage() {
	fmt.Printf(usage, os.Args[0], os.Args[0])
}

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

func initSuperuser(cfgFile, email, password, fullName string) {
	cfg, err := getConfig(cfgFile)
	if err != nil {
		log.Errorf("validating config: %v", err)
		os.Exit(1)
	}
	userMgr, err := admin.GetUserManager(cfg.Database, cfg.Default)
	if err != nil {
		log.Errorf("getting user manager: %v", err)
		os.Exit(1)
	}
	userParams := params.NewUserParams{
		Email:    email,
		Password: password,
		FullName: fullName,
	}
	if _, err := userMgr.CreateSuperUser(userParams); err != nil {
		log.Errorf("failed to create super user: %v", err)
		os.Exit(1)
	}
	return
}

func runAPIServer(cfgFile string) {
	stop := make(chan os.Signal)
	signal.Notify(stop, syscall.SIGTERM)
	signal.Notify(stop, syscall.SIGINT)
	log.SetLogLevel(loggo.DEBUG)

	cfg, err := getConfig(cfgFile)
	if err != nil {
		log.Errorf("error validating config: %q", err)
		os.Exit(1)
	}
	apiServer, err := apiserver.GetAPIServer(cfg)
	if err != nil {
		log.Errorf("error getting apiserver: %v", err)
		os.Exit(1)
	}
	if err := apiServer.Start(); err != nil {
		log.Errorf("error starting api worker: %v", err)
		os.Exit(1)
	}
	select {
	case <-stop:
		log.Infof("shutting down gracefully")
		apiServer.Stop()
	}
}

func main() {
	firstRunCmd := flag.NewFlagSet("init", flag.ExitOnError)
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)

	firstRunCmdCfgFile := firstRunCmd.String("config", "", "gopherbin config file")
	firstRunCmdEmail := firstRunCmd.String("email", "", "super user email")
	firstRunCmdPassword := firstRunCmd.String("password", "", "super user passsword")
	firstRunCmdFullName := firstRunCmd.String("fullName", "", "super user full name")

	runCmdCfgFile := runCmd.String("config", "", "gopherbin config file")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		firstRunCmd.Parse(os.Args[2:])
		if *firstRunCmdCfgFile == "" || *firstRunCmdEmail == "" || *firstRunCmdPassword == "" || *firstRunCmdFullName == "" {
			firstRunCmd.PrintDefaults()
			os.Exit(1)
		}
		initSuperuser(
			*firstRunCmdCfgFile,
			*firstRunCmdEmail,
			*firstRunCmdPassword,
			*firstRunCmdFullName)
	case "run":
		runCmd.Parse(os.Args[2:])
		if *runCmdCfgFile == "" {
			runCmd.PrintDefaults()
			os.Exit(1)
		}
		runAPIServer(*runCmdCfgFile)
	default:
		fmt.Printf("%q is not valid command.\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}
