package worker

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	"github.com/struCoder/pidusage"
	"github.com/trevor403/gostream/pkg/ipcmsg"
)

// MessageHandler function definition
type MessageHandler func(data []byte)

// type MessageHandler func(msg ipcmsg.IPCMessage, w chan ipcmsg.IPCMessage)

func run(handler MessageHandler) {
	for {
		pid, r, w := fork()
		_ = w
		// ticker := time.NewTicker(5 * time.Second)
		// quit := make(chan struct{})
		// go func() {
		// 	for {
		// 		select {
		// 		case <-ticker.C:
		// 			w <- ipcmsg.Message(42, []byte{42})
		// 		case <-quit:
		// 			ticker.Stop()
		// 			return
		// 		}
		// 	}
		// }()

		fmt.Println("child pid", pid)
		proc, err := os.FindProcess(pid)
		if err != nil {
			syscall.Kill(pid, syscall.SIGKILL)
			continue
		}

		done := make(chan bool, 1)
		go func() {
			// returns after the proc ends
			state, _ := proc.Wait()

			fmt.Println("child exited", state.ExitCode())
			done <- true
		}()

		leak := make(chan bool, 1)
		go func() {
			// wait until the process is running
			time.Sleep(time.Second)

			// check status
			for {
				sysInfo, err := pidusage.GetStat(pid)
				if err != nil {
					break
				}
				currentBytes := sysInfo.Memory
				if currentBytes > maxMemBytes {
					leak <- true
					break
				}
				time.Sleep(5 * time.Second)
			}
		}()

	msgloop:
		for {
			select {
			case msg := <-r:
				handler(msg.Data)
			case <-leak:
				fmt.Println("child mem kill")
				proc.Kill()
				break msgloop
			case <-done:
				break msgloop
			}
		}

		close(r)
		close(w)
		// close(quit)

		fmt.Println("restarting child")

		// TODO: invalidate any in-flight requests
	}
}

func fork() (pid int, r chan ipcmsg.IPCMessage, w chan ipcmsg.IPCMessage) {
	sp, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, syscall.AF_UNSPEC)
	if err != nil {
		log.Fatalf("sockpair err: %s", err)
	}

	forkNum := 0
	args := os.Args
	childENV := []string{
		fmt.Sprintf("%s=%d", childEnvName, forkNum),
	}
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd err: %s", err)
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("getexe err: %s", err)
	}
	childPID, err := syscall.ForkExec(exe, args, &syscall.ProcAttr{
		Dir: pwd,
		Env: append(os.Environ(), childENV...),
		Sys: &syscall.SysProcAttr{
			Setsid: true,
			// Pdeathsig: syscall.SIGPIPE,
		},
		Files: []uintptr{
			uintptr(syscall.Stdin),
			uintptr(syscall.Stdout),
			uintptr(syscall.Stderr),
			uintptr(sp[0]),
		},
	})
	if err != nil {
		log.Fatalf("fork err: %s", err)
	}

	if syscall.Close(sp[0]) != nil {
		log.Fatalf("close err: %s", err)
	}

	child_r, child_w := ipcmsg.Channel(childPID, sp[1])

	return childPID, child_r, child_w
}

func watch_parent() {
	ppid := os.Getppid()

	// check status
	for {
		err := syscall.Kill(ppid, syscall.Signal(0))
		if err == syscall.ESRCH {
			os.Exit(128)
		}
		time.Sleep(time.Second)
	}
}
