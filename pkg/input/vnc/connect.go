package gostream

import (
	"fmt"
	"net"

	"github.com/polyisobutylene/go-vnc"
)

const qemuHostIP = "192.168.55.1"

// VNCInfo ...
type VNCInfo struct {
	Port     int
	Password string
}

func connectVNC() {
	port := 5900
	password := ""
	address := fmt.Sprintf("%s:%d", qemuHostIP, port)

	nc, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Printf("Error connecting to VNC: %s", err)
	}
	defer nc.Close()

	var auth vnc.ClientAuth
	if password != "" {
		auth = &vnc.PasswordAuth{Password: password}
	} else {
		auth = new(vnc.ClientAuthNone)
	}

	c, err := vnc.Client(nc, &vnc.ClientConfig{
		Auth: []vnc.ClientAuth{auth},
	})
	if err != nil {
		fmt.Printf("Error handshaking with VNC: %s", err)
	}
	defer c.Close()
	fmt.Printf("Connected to VNC desktop: %s [res:%dx%d]\n", c.DesktopName, c.FrameBufferWidth, c.FrameBufferHeight)
}
