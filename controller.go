package goControlTor

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net"
	"net/textproto"
	"path"
)

const (
	cmdOK           = 250
	cmdAuthenticate = "AUTHENTICATE"
)

type TorControl struct {
	netConn                net.Conn
	textprotoReader        *textproto.Reader
	authenticationPassword string
}

func (t *TorControl) Dial(network, addr string) error {
	var err error = nil
	t.netConn, err = net.Dial(network, addr)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(t.netConn)
	t.textprotoReader = textproto.NewReader(reader)
	return nil
}

func (t *TorControl) SendCommand(command string) error {
	_, err := t.netConn.Write([]byte(command))
	if err != nil {
		return fmt.Errorf("writing to tor control port: %s", err)
	}
	_, _, err = t.textprotoReader.ReadCodeLine(cmdOK)
	if err != nil {
		return fmt.Errorf("reading tor control port command status: %s", err)
	}
	return nil
}

func (t *TorControl) PasswordAuthenticate(password string) error {
	authCmd := fmt.Sprintf("%s \"%s\"\n", cmdAuthenticate, password)
	return t.SendCommand(authCmd)
}

// Creates a Tor Hidden Service with the HiddenServiceDirGroupReadable option
// set so that the service's hostname file will have group read permission set.
// At this time of writing this feature is only available in the alpha version
// of tor. See https://trac.torproject.org/projects/tor/ticket/11291
func (t *TorControl) CreateHiddenService(serviceDir string, listenAddrs map[int]string) error {
	var createCmd string = fmt.Sprintf("SETCONF hiddenservicedir=%s", serviceDir)
	for virtPort, listenAddr := range listenAddrs {
		createCmd += fmt.Sprintf(" hiddenserviceport=\"%d %s\"", virtPort, listenAddr)
	}
	createCmd += " HiddenServiceDirGroupReadable=1\n"
	return t.SendCommand(createCmd)
}

func (t *TorControl) DeleteHiddenService(serviceDir string) error {
	var deleteCmd string = fmt.Sprintf("SETCONF hiddenservicedir=%s\n", serviceDir)
	return t.SendCommand(deleteCmd)
}

func ReadOnion(serviceDir string) (string, error) {
	onion, err := ioutil.ReadFile(path.Join(serviceDir, "hostname"))
	if err != nil {
		return "", fmt.Errorf("reading Tor hidden service hostname file: %s", err)
	}
	return string(onion), nil
}
