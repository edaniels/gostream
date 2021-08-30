package input

const (
	KeyEventType   = "keypress"
	MouseEventType = "mouse"
)

type RawEvent struct {
	Type string `json:"type"`
}

type KeyEvent struct {
	RawEvent     // inherit
	Key      int `json:"key"`
	State    int `json:"state"`
}

type MouseEvent struct {
	RawEvent       // inherit
	X          int `json:"x"`
	Y          int `json:"y"`
	ButtonMask int `json:"mask"`
}
