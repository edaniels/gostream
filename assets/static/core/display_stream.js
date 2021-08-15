import * as Log from './util/logging.js';
import { supportsImageMetadata } from './util/browser.js';
import { toSigned32bit } from './util/int.js';

const start_stream = async function(target, cursorHandler, _set_scale_handler) {
    const peerConnection = new RTCPeerConnection({
        iceServers: window.iceServers
    });

    peerConnection.ontrack = function (event) {
        const id = event.streams[0].id;
        console.log("track for stream", id);
        const videoElement = target;
        videoElement.srcObject = event.streams[0];
        videoElement.autoplay = true;
        videoElement.controls = false;
        videoElement.playsInline = true;
        videoElement.onclick = event => { };
    }

    peerConnection.onicecandidate = async function (event) {
        if (event.candidate !== null) {
            return;
        }
        const response = await fetch("/offer_0", {
            method: 'POST',
            mode: 'cors',
            body: btoa(JSON.stringify(peerConnection.localDescription))
        });
        const text = await response.text();
        if (response.status != 200) {
            if (text.length !== 0) {
                console.log(text);
            } else {
                console.log(response.statusText);
            }
            return;
        }
        try {
            peerConnection.setRemoteDescription(new RTCSessionDescription(JSON.parse(atob(text))));
        } catch (e) {
            console.log(e);
        }
    }
    peerConnection.onsignalingstatechange = () => console.log(peerConnection.signalingState);
    peerConnection.oniceconnectionstatechange = () => console.log(peerConnection.iceConnectionState);

    // set up offer
    const cursorChannel = peerConnection.createDataChannel("cursor", { negotiated: true, id: 2 });
    const resizeChannel = peerConnection.createDataChannel("resize", { negotiated: true, id: 1 });
    const dataChannel = peerConnection.createDataChannel("data", { negotiated: true, id: 0 });
    cursorChannel.onmessage = cursorHandler;
    resizeChannel.onmessage = function (event) {
        console.log("resizeChannel", event.data);
    }
    dataChannel.onmessage = function (event) {
        console.log("dataChannel", event.data);
    }

    peerConnection.addTransceiver('video', { 'direction': 'sendrecv' });

    let doubleToByteArray = function(number) {
        var buffer = new ArrayBuffer(4);
        var floatView = new Float32Array(buffer);

        floatView[0] = number;

        return buffer;
    }
    _set_scale_handler(function(scale) {
        if (scale == 0) return;
        if (resizeChannel.readyState == "open") {
            let buf = doubleToByteArray(scale);
            resizeChannel.send(buf);
        }
    });

    const offerDesc = await peerConnection.createOffer();
    let set = false;
    try {
        peerConnection.setLocalDescription(offerDesc)
        set = true;
    } catch (e) {
        console.log(e);
    }

    if (!set) {
        return;
    }

    // dataChannel.send(textInput.value);
}

export default class DisplayStream {
    constructor(target, updateCursor) {
        // the full frame buffer (logical canvas) size
        this._fbWidth = 0;
        this._fbHeight = 0;

        // this._updateCursor(rgba, hotx, hoty, w, h);
        this._updateCursor = updateCursor;

        Log.Debug(">> DisplayStream.constructor");

        // The visible canvas
        this._target = target;

        if (!this._target) {
            throw new Error("Target must be set");
        }

        if (typeof this._target === 'string') {
            throw new Error('target must be a DOM element');
        }

        var _cursorHandler = function (event) {
            // https://github.com/photopea/UPNG.js
            var raw = new Uint8Array(event.data, 0, 4);
            var imageData = new Uint8Array(event.data, 4);
            let w = raw[0];
            let h = raw[1];
            let hotx = raw[2];
            let hoty = raw[3];

            this._updateCursor(imageData, hotx, hoty, w, h);
        }

        this._scale_handler = function(scale) {};
        start_stream(target, _cursorHandler.bind(this), this._set_scale_handler.bind(this));

        // the visible canvas viewport (i.e. what actually gets seen)
        this._viewportLoc = { 'x': 0, 'y': 0, 'w': this._target.width, 'h': this._target.height };

        Log.Debug("<< DisplayStream.constructor");

        // ===== PROPERTIES =====

        this._scale = 1.0;
        this._clipViewport = false;
    }

    // ===== PROPERTIES =====

    get scale() { return this._scale; }
    set scale(scale) {
        this._rescale(scale);
    }

    get clipViewport() { return this._clipViewport; }
    set clipViewport(viewport) {
        this._clipViewport = viewport;
        // May need to readjust the viewport dimensions
        const vp = this._viewportLoc;
        this.viewportChangeSize(vp.w, vp.h);
        this.viewportChangePos(0, 0);
    }

    get width() {
        return this._fbWidth;
    }

    get height() {
        return this._fbHeight;
    }

    // ===== PUBLIC METHODS =====

    updateCursor() {
        const bytesPerPixel = 4;

        let rgba = new Uint8Array(w * h * bytesPerPixel);

        this._updateCursor(rgba, hotx, hoty, w, h);
    }

    viewportChangePos(deltaX, deltaY) {
        const vp = this._viewportLoc;
        deltaX = Math.floor(deltaX);
        deltaY = Math.floor(deltaY);

        if (!this._clipViewport) {
            deltaX = -vp.w;  // clamped later of out of bounds
            deltaY = -vp.h;
        }

        const vx2 = vp.x + vp.w - 1;
        const vy2 = vp.y + vp.h - 1;

        // Position change

        if (deltaX < 0 && vp.x + deltaX < 0) {
            deltaX = -vp.x;
        }
        if (vx2 + deltaX >= this._fbWidth) {
            deltaX -= vx2 + deltaX - this._fbWidth + 1;
        }

        if (vp.y + deltaY < 0) {
            deltaY = -vp.y;
        }
        if (vy2 + deltaY >= this._fbHeight) {
            deltaY -= (vy2 + deltaY - this._fbHeight + 1);
        }

        if (deltaX === 0 && deltaY === 0) {
            return;
        }
        Log.Debug("viewportChange deltaX: " + deltaX + ", deltaY: " + deltaY);

        vp.x += deltaX;
        vp.y += deltaY;
    }

    viewportChangeSize(width, height) {
        if (!this._clipViewport ||
            typeof (width) === "undefined" ||
            typeof (height) === "undefined") {

            Log.Debug("Setting viewport to full display region");
            width = this._fbWidth;
            height = this._fbHeight;
        }

        width = Math.floor(width);
        height = Math.floor(height);

        if (width > this._fbWidth) {
            width = this._fbWidth;
        }
        if (height > this._fbHeight) {
            height = this._fbHeight;
        }

        const vp = this._viewportLoc;
        if (vp.w !== width || vp.h !== height) {
            vp.w = width;
            vp.h = height;

            const canvas = this._target;
            canvas.width = width;
            canvas.height = height;

            // The position might need to be updated if we've grown
            this.viewportChangePos(0, 0);

            // Update the visible size of the target canvas
            this._rescale(this._scale);
        }
    }

    absX(x) {
        if (this._scale === 0) {
            return 0;
        }
        return toSigned32bit(x / this._scale + this._viewportLoc.x);
    }

    absY(y) {
        if (this._scale === 0) {
            return 0;
        }
        return toSigned32bit(y / this._scale + this._viewportLoc.y);
    }

    resize(width, height) {
        this._prevDrawStyle = "";

        this._fbWidth = width;
        this._fbHeight = height;

        // Readjust the viewport as it may be incorrectly sized
        // and positioned
        const vp = this._viewportLoc;
        this.viewportChangeSize(vp.w, vp.h);
        this.viewportChangePos(0, 0);
    }


    autoscale(containerWidth, containerHeight) {
        let scaleRatio;

        if (containerWidth === 0 || containerHeight === 0) {
            scaleRatio = 0;

        } else {

            const vp = this._viewportLoc;
            const targetAspectRatio = containerWidth / containerHeight;
            const fbAspectRatio = vp.w / vp.h;

            if (fbAspectRatio >= targetAspectRatio) {
                scaleRatio = containerWidth / vp.w;
            } else {
                scaleRatio = containerHeight / vp.h;
            }
        }

        this._rescale(scaleRatio);
    }

    // ===== PRIVATE METHODS =====

    _set_scale_handler(handler) {
        this._scale_handler = handler;
    }

    _rescale(factor) {
        this._scale_handler(factor);
        this._scale = factor;
        const vp = this._viewportLoc;

        // NB(directxman12): If you set the width directly, or set the
        //                   style width to a number, the canvas is cleared.
        //                   However, if you set the style width to a string
        //                   ('NNNpx'), the canvas is scaled without clearing.
        const width = factor * vp.w + 'px';
        const height = factor * vp.h + 'px';

        if ((this._target.style.width !== width) ||
            (this._target.style.height !== height)) {
            this._target.style.width = width;
            this._target.style.height = height;
        }
    }
}
