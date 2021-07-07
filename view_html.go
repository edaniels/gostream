package gostream

// ViewHTML is the HTML needed to interact with the view in a browser.
type ViewHTML struct {
	JavaScript string
	Body       string
}

func (bv *basicView) SinglePageHTML() string {
	// bv.streamNum()
	// bv.numReservedStreams()
	return bv.iceServers()
}

func (bv *basicView) HTML() ViewHTML {
	return ViewHTML{
		// JavaScript: fmt.Sprintf(viewJS, bv.htmlArgs()...),
		// Body:       fmt.Sprintf(viewBody, bv.htmlArgs()...),
	}
}
