package worker

import (
	"fmt"
	"path"
	"reflect"
	"strings"
)

// const childEnvName = "CHILD_ID"

type Empty struct{}

var pkgName = path.Base(reflect.TypeOf(Empty{}).PkgPath())

var childEnvName = fmt.Sprintf("%s_CHILD_ID", strings.ToUpper(pkgName))
