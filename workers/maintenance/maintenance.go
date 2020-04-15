package maintenance

import (
	"fmt"
	"gopherbin/admin"
	admCommon "gopherbin/admin/common"
	"gopherbin/config"
	"gopherbin/workers/common"
	"time"

	"github.com/juju/loggo"
)

var _ = (common.Worker)(&maintenanceWorker{})
var log = loggo.GetLogger("gopherbin.workers.maintenance")

// NewMaintenanceWorker returns a new maintenance worker
func NewMaintenanceWorker(cfg config.Database, defCfg config.Default) (common.Worker, error) {
	mgr, err := admin.GetUserManager(cfg, defCfg)
	if err != nil {
		return nil, err
	}
	return &maintenanceWorker{
		mgr:     mgr,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}, nil
}

type maintenanceWorker struct {
	mgr     admCommon.UserManager
	stop    chan struct{}
	stopped chan struct{}
}

func (m *maintenanceWorker) Start() error {
	go m.loop()
	return nil
}

func (m *maintenanceWorker) Stop() error {
	close(m.stop)
	select {
	case <-m.stopped:
		return nil
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("time out waiting for maintenance worker to exit")
	}
	return nil
}

func (m *maintenanceWorker) loop() {
	for {
		select {
		case <-time.After(10 * time.Minute):
			log.Infof("cleaning token blacklist")
			if err := m.mgr.CleanTokens(); err != nil {
				log.Warningf("error cleaning tokens: %q", err)
			}
		case <-m.stop:
			defer close(m.stopped)
			return
		}
	}
}
