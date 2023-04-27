package gostream

import (
	"testing"

	"go.viam.com/test"
)

func TestParseName(t *testing.T) {
	prettyName := "Dummy video device (0x0000) (platform:v4l2loopback-000)"
	name, id := parseNameAndID(prettyName)
	test.That(t, name, test.ShouldEqual, "Dummy video device (0x0000)")
	test.That(t, id, test.ShouldEqual, "platform:v4l2loopback-000")

	prettyName = "Mac OS X: FaceTime HD Camera (Built-in) (0x1420000005ac8600)"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldEqual, "Mac OS X: FaceTime HD Camera (Built-in)")
	test.That(t, id, test.ShouldEqual, "0x1420000005ac8600")

	prettyName = "Linux: Laptop Camera: Laptop Camera (usb-0000:00:14.0-7)"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldEqual, "Linux: Laptop Camera: Laptop Camera")
	test.That(t, id, test.ShouldEqual, "usb-0000:00:14.0-7")

	prettyName = "Linux: Laptop Camera: Laptop Camera (video0)"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldEqual, "Linux: Laptop Camera: Laptop Camera")
	test.That(t, id, test.ShouldEqual, "video0")

	prettyName = "ERROR: camera name ok but no parenthesis "
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldBeZeroValue)
	test.That(t, id, test.ShouldBeZeroValue)

	prettyName = "ERROR: camera name ok but no ID ()"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldBeZeroValue)
	test.That(t, id, test.ShouldBeZeroValue)

	prettyName = " (ERROR: ID ok but no name)"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldBeZeroValue)
	test.That(t, id, test.ShouldBeZeroValue)

	prettyName = "ERROR: camera name ok but (ID has no close parenthesis"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldBeZeroValue)
	test.That(t, id, test.ShouldBeZeroValue)

	prettyName = "ERROR: camera name ok but ID has no open parenthesis)"
	name, id = parseNameAndID(prettyName)
	test.That(t, name, test.ShouldBeZeroValue)
	test.That(t, id, test.ShouldBeZeroValue)
}
