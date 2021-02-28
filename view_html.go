package gostream

import "fmt"

// ViewHTML is the HTML needed to interact with the view in a browser.
type ViewHTML struct {
	JavaScript string
	Body       string
}

func (bv *basicView) SinglePageHTML() string {
	return fmt.Sprintf(viewSingleHTML, bv.htmlArgs()...)
}

func (bv *basicView) HTML() ViewHTML {
	return ViewHTML{
		JavaScript: fmt.Sprintf(viewJS, bv.htmlArgs()...),
		Body:       fmt.Sprintf(viewBody, bv.htmlArgs()...),
	}
}

var viewSingleHTML = `
<!DOCTYPE html>
<html>
<head>
  <script type="text/javascript">
  ` + viewJS + `
  </script>
</head>
<body>
` + viewBody + `
</body>
</html>
`

var viewJS = `
const start_%[2]d = function() {
  let peerConnection = new RTCPeerConnection({
    iceServers: %[4]s
  });

  const calculateClick = (el, event) => {
    // https://stackoverflow.com/a/288731/1497139
    bounds = el.getBoundingClientRect();
    let left = bounds.left;
    let top = bounds.top;
    let x = event.pageX - left;
    let y = event.pageY - top;
    let cw = el.clientWidth;
    let ch = el.clientHeight;
    let iw = el.videoWidth;
    let ih = el.videoHeight;
    let px = Math.min(x / cw * iw, el.videoWidth-1);
    let py = Math.min(y / ch * ih, el.videoHeight-1);
    return {x: px, y: py};
  }

  peerConnection.ontrack = event => {
    var id = event.streams[0].id;
    var containerElement = document.createElement("div");
    var videoElement = document.createElement(event.track.kind);
    videoElement.srcObject = event.streams[0];
    videoElement.autoplay = true;
    videoElement.controls = false;
    videoElement.playsInline = true;
    videoElement.onclick = event => {
      coords = calculateClick(videoElement, event);
      clickChannel.send(coords.x + "," + coords.y);
    }
    var textElement = document.createElement("div");
    textElement.textContent = id;
    containerElement.setAttribute("id", id);
    containerElement.appendChild(textElement);
    containerElement.appendChild(videoElement);
    document.getElementById('remoteVideo_%[2]d').appendChild(containerElement);
  }

  peerConnection.onicecandidate = event => {
    if (event.candidate !== null) {
      return;
    }
    fetch("/offer_%[2]d", {
      method: 'POST',
      mode: 'cors',
      body: btoa(JSON.stringify(peerConnection.localDescription))
    })
    .then(response => response.text())
    .then(text => {
      try {
        peerConnection.setRemoteDescription(new RTCSessionDescription(JSON.parse(atob(text))));
      } catch(e) {
        console.log(e);
      }
    });
  }
  peerConnection.onsignalingstatechange = () => console.log(peerConnection.signalingState);
  peerConnection.oniceconnectionstatechange = () => console.log(peerConnection.iceConnectionState);

  // set up offer
  let clickChannel = peerConnection.createDataChannel("clicks", {negotiated: true, id: 1});
  let dataChannel = peerConnection.createDataChannel("data", {negotiated: true, id: 0});
  clickChannel.onmessage = function(event) {
    console.log(event.data);
  }
  dataChannel.onmessage = function(event) {
    console.log(event.data);
  }
  for (var i = 0; i < %[3]d; i++) {
    peerConnection.addTransceiver('video', {'direction': 'sendrecv'});
  }
  peerConnection.createOffer()
    .then(desc => peerConnection.setLocalDescription(desc))
    .catch(console.log);

  var textInput = document.createElement("input");
  textInput.setAttribute("type", "text");
  textInput.onkeydown = function(event) {
    if(event.key !== 'Enter') {
      return;
    }
    if (textInput.value === "") {
      return;
    }
    dataChannel.send(textInput.value);
  }
  document.getElementById("stream_%[2]d").prepend(textInput);
}
`

var viewBody = `
View<br />
<button onclick="start_%[2]d(); this.remove();">Start%[1]s</button>
<div id="stream_%[2]d">
  <div id="remoteVideo_%[2]d"></div><br />
</div>
<br />
`
