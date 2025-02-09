package service

import (
	"fmt"

	"github.com/kardianos/osext"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func newService(c *Config) (*windowsService, error) {
	return &windowsService{
		name:        c.Name,
		displayName: c.DisplayName,
		description: c.Description,
	}, nil
}

type windowsService struct {
	name, displayName, description string
	onStart, onStop                func() error
	logger                         *eventlog.Log
}

const version = "Windows Service"

func (ws *windowsService) String() string {
	return version
}

func (ws *windowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	if err := ws.onStart(); err != nil {
		ws.Error(err.Error())
		return true, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
loop:
	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			if err := ws.onStop(); err != nil {
				ws.Error(err.Error())
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
				continue loop
			}
			break loop
		default:
			continue loop
		}
	}

	return
}

func (ws *windowsService) Install() error {
	exepath, err := osext.Executable()
	if err != nil {
		return err
	}
	// Used if path contains a space.
	//exepath = '"' + exepath + '"'
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ws.name)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", ws.name)
	}
	s, err = m.CreateService(ws.name, exepath, mgr.Config{
		DisplayName: ws.displayName,
		Description: ws.description,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(ws.name, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("InstallAsEventCreate() failed: %s", err)
	}
	return nil
}

func (ws *windowsService) Remove() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ws.name)
	if err != nil {
		return fmt.Errorf("service %s is not installed", ws.name)
	}
	defer s.Close()
	err = s.Delete()
	if err != nil {
		return err
	}
	err = eventlog.Remove(ws.name)
	if err != nil {
		return fmt.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return nil
}

func (ws *windowsService) Run(onStart, onStop func() error) error {
	elog, err := eventlog.Open(ws.name)
	if err != nil {
		return err
	}
	defer elog.Close()

	ws.logger = elog

	ws.onStart = onStart
	ws.onStop = onStop
	return svc.Run(ws.name, ws)
}

func (ws *windowsService) Start() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.name)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Start()
}

func (ws *windowsService) Stop() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(ws.name)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.Control(svc.Stop)
	return err
}

func (s *windowsService) IntervalMode(interval int) error {
	return fmt.Errorf("interval mode service only supported on darwin")
}

func (ws *windowsService) Error(format string, a ...interface{}) error {
	if ws.logger == nil {
		return nil
	}
	return ws.logger.Error(3, fmt.Sprintf(format, a...))
}
func (ws *windowsService) Warning(format string, a ...interface{}) error {
	if ws.logger == nil {
		return nil
	}
	return ws.logger.Warning(2, fmt.Sprintf(format, a...))
}
func (ws *windowsService) Info(format string, a ...interface{}) error {
	if ws.logger == nil {
		return nil
	}
	return ws.logger.Info(1, fmt.Sprintf(format, a...))
}
