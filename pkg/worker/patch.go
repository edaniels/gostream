package worker

import (
	"os"

	_ "unsafe"

	"bou.ke/monkey"
)

//go:linkname main_main main.main
func main_main()

func init() {
	if _, isChild := os.LookupEnv(ChildEnvName); isChild {
		monkey.Patch(main_main, fork_main)
		go watch_parent()
	}
}
