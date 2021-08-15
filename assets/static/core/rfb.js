// noLatency: HTML5 WebRTC Desktop client

import { toUnsigned32bit, toSigned32bit } from './util/int.js';
import * as Log from './util/logging.js';
import { encodeUTF8, decodeUTF8 } from './util/strings.js';
import { dragThreshold } from './util/browser.js';
import { clientToElement } from './util/element.js';
import { setCapture } from './util/events.js';
import EventTargetMixin from './util/eventtarget.js';
import Display from "./display.js";
import DisplayStream from "./display_stream.js";
import Keyboard from "./input/keyboard.js";
import GestureHandler from "./input/gesturehandler.js";
import Cursor from "./util/cursor.js";
import KeyTable from "./input/keysym.js";
import XtScancode from "./input/xtscancodes.js";
import "./util/polyfill.js";

// How many seconds to wait for a disconnect to finish
const DISCONNECT_TIMEOUT = 3;
const DEFAULT_BACKGROUND = 'rgb(40, 40, 40)';

// Minimum wait (ms) between two mouse moves
const MOUSE_MOVE_DELAY = 17;

// Wheel thresholds
const WHEEL_STEP = 50; // Pixels needed for one step
const WHEEL_LINE_HEIGHT = 19; // Assumed pixels for one line step

// Gesture thresholds
const GESTURE_ZOOMSENS = 75;
const GESTURE_SCRLSENS = 50;
const DOUBLE_TAP_TIMEOUT = 1000;
const DOUBLE_TAP_THRESHOLD = 50;

export default class TSD extends EventTargetMixin {
    constructor(target, url, options) {
        if (!target) {
            throw new Error("Must specify target");
        }
        if (!url) {
            throw new Error("Must specify URL");
        }

        super();

        this._target = target;
        this._url = url;

        // Connection details
        options = options || {};
        this._tsdCredentials = options.credentials || {};
        this._shared = 'shared' in options ? !!options.shared : true;
        this._repeaterID = options.repeaterID || '';
        this._wsProtocols = options.wsProtocols || [];

        // Internal state
        this._tsdConnectionState = '';
        this._tsdInitState = '';
        this._tsdAuthScheme = -1;
        this._tsdCleanDisconnect = true;

        // Server capabilities
        this._tsdVersion = 0;

        this._fbWidth = 0;
        this._fbHeight = 0;

        this._fbName = "";

        this._capabilities = { power: false };

        this._supportsFence = false;

        this._supportsContinuousUpdates = false;
        this._enabledContinuousUpdates = false;

        this._screenID = 0;
        this._screenFlags = 0;

        this._clipboardText = null;

        // Internal objects
        this._sock = null;              // Websock object
        this._display = null;           // Display object
        this._display_webrtc = null;
        this._flushing = false;         // Display flushing state
        this._keyboard = null;          // Keyboard input handler object
        this._gestures = null;          // Gesture input handler object

        // Timers
        this._disconnTimer = null;      // disconnection timer
        this._resizeTimeout = null;     // resize rate limiting
        this._mouseMoveTimer = null;

        // Decoder states
        this._decoders = {};

        this._FBU = {
            rects: 0,
            x: 0,
            y: 0,
            width: 0,
            height: 0,
        };

        // Mouse state
        this._mousePos = {};
        this._mouseButtonMask = 0;
        this._mouseLastMoveTime = 0;
        this._viewportDragging = false;
        this._viewportDragPos = {};
        this._viewportHasMoved = false;
        this._accumulatedWheelDeltaX = 0;
        this._accumulatedWheelDeltaY = 0;

        // Gesture state
        this._gestureLastTapTime = null;
        this._gestureFirstDoubleTapEv = null;
        this._gestureLastMagnitudeX = 0;
        this._gestureLastMagnitudeY = 0;

        // Bound event handlers
        this._eventHandlers = {
            focusCanvas: this._focusCanvas.bind(this),
            windowResize: this._windowResize.bind(this),
            handleMouse: this._handleMouse.bind(this),
            handleWheel: this._handleWheel.bind(this),
            handleGesture: this._handleGesture.bind(this),
        };

        // main setup
        Log.Debug(">> TSD.constructor");

        // Create DOM elements
        this._screen = document.createElement('div');
        this._screen.style.display = 'grid';
        this._screen.style.width = '100%';
        this._screen.style.height = '100%';
        this._screen.style.overflow = 'auto';
        this._screen.style.background = DEFAULT_BACKGROUND;
    
        this._canvas = document.createElement('canvas');
        this._canvas.style.margin = 'auto';
        // Some browsers add an outline on focus
        this._canvas.style.outline = 'none';
        this._canvas.style.gridRow = 1;
        this._canvas.style.gridColumn = 1;
        this._canvas.style.zIndex = 1;
        this._canvas.width = 0;
        this._canvas.height = 0;
        this._canvas.tabIndex = -1;
        this._screen.appendChild(this._canvas);

        this._stream = document.createElement('video');
        this._stream.style.margin = 'auto';
        // Some browsers add an outline on focus
        this._stream.style.outline = 'none';
        this._stream.style.gridRow = 1;
        this._stream.style.gridColumn = 1;
        this._stream.width = 0;
        this._stream.height = 0;
        this._stream.tabIndex = -1;
        this._screen.appendChild(this._stream);

        // Test
        // this._canvas.style.cursor = 'url("data:image/x-icon;base64,AAACAAEACAgAAAIAAgA4AQAAFgAAACgAAAAIAAAAEAAAAAEAIAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAD/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////AAAAAAAAAAAAAAAAAAAAAA==") 2 2, default';
        // this._stream.autoplay = true;
        // this._stream.src = "https://cdn.videvo.net/videvo_files/video/free/2019-01/small_watermarked/181015_16b_Venice%20Fishing%20Pier_HD_016_preview.webm";

        // Cursor
        this._cursor = new Cursor();

        // XXX: TightVNC 2.8.11 sends no cursor at all until Windows changes
        // it. Result: no cursor at all until a window border or an edit field
        // is hit blindly. But there are also VNC servers that draw the cursor
        // in the framebuffer and don't send the empty local cursor. There is
        // no way to satisfy both sides.
        //
        // The spec is unclear on this "initial cursor" issue. Many other
        // viewers (TigerVNC, RealVNC, Remmina) display an arrow as the
        // initial cursor instead.
        this._cursorImage = TSD.cursors.none;

        // NB: nothing that needs explicit teardown should be done
        // before this point, since this can throw an exception
        try {
            this._display = new Display(this._canvas);
            this._display_webrtc = new DisplayStream(this._stream, this._updateCursor.bind(this));
        } catch (exc) {
            Log.Error("Display exception: " + exc);
            throw exc;
        }
        this._display.onflush = this._onFlush.bind(this);

        this._keyboard = new Keyboard(this._canvas);
        this._keyboard.onkeyevent = this._handleKeyEvent.bind(this);

        this._gestures = new GestureHandler();

        this._sock = new Websock();
        this._sock.on('message', () => {
            this._handleMessage();
        });
        this._sock.on('open', () => {
            if ((this._tsdConnectionState === 'connecting') &&
                (this._tsdInitState === '')) {
                this._tsdInitState = 'ProtocolVersion';
                Log.Debug("Starting VNC handshake");
            } else {
                this._fail("Unexpected server connection while " +
                           this._tsdConnectionState);
            }
        });
        this._sock.on('close', (e) => {
            Log.Debug("WebSocket on-close event");
            let msg = "";
            if (e.code) {
                msg = "(code: " + e.code;
                if (e.reason) {
                    msg += ", reason: " + e.reason;
                }
                msg += ")";
            }
            switch (this._tsdConnectionState) {
                case 'connecting':
                    this._fail("Connection closed " + msg);
                    break;
                case 'connected':
                    // Handle disconnects that were initiated server-side
                    this._updateConnectionState('disconnecting');
                    this._updateConnectionState('disconnected');
                    break;
                case 'disconnecting':
                    // Normal disconnection path
                    this._updateConnectionState('disconnected');
                    break;
                case 'disconnected':
                    this._fail("Unexpected server disconnect " +
                               "when already disconnected " + msg);
                    break;
                default:
                    this._fail("Unexpected server disconnect before connecting " +
                               msg);
                    break;
            }
            this._sock.off('close');
        });
        this._sock.on('error', e => Log.Warn("WebSocket on-error event"));

        // Slight delay of the actual connection so that the caller has
        // time to set up callbacks
        setTimeout(this._updateConnectionState.bind(this, 'connecting'));

        console.log(Log.Debug)
        Log.Debug("<< TSD.constructor");

        // ===== PROPERTIES =====

        this.dragViewport = false;
        this.focusOnClick = true;

        this._viewOnly = false;
        this._clipViewport = false;
        this._scaleViewport = false;
        this._resizeSession = false;

        this._showDotCursor = false;
        if (options.showDotCursor !== undefined) {
            Log.Warn("Specifying showDotCursor as a TSD constructor argument is deprecated");
            this._showDotCursor = options.showDotCursor;
        }
    }

    // ===== PROPERTIES =====

    get viewOnly() { return this._viewOnly; }
    set viewOnly(viewOnly) {
        this._viewOnly = viewOnly;

        if (this._tsdConnectionState === "connecting" ||
            this._tsdConnectionState === "connected") {
            if (viewOnly) {
                this._keyboard.ungrab();
            } else {
                this._keyboard.grab();
            }
        }
    }

    get scaleViewport() { return this._scaleViewport; }
    set scaleViewport(scale) {
        this._scaleViewport = scale;
    }

    get resizeSession() { return this._resizeSession; }
    set resizeSession(resize) {
        this._resizeSession = resize;
    }

    get showDotCursor() { return this._showDotCursor; }
    set showDotCursor(show) {
        this._showDotCursor = show;
        this._refreshCursor();
    }

    get background() { return this._screen.style.background; }
    set background(cssValue) { this._screen.style.background = cssValue; }

    // ===== PUBLIC METHODS =====

    disconnect() {
        this._updateConnectionState('disconnecting');
        this._sock.off('error');
        this._sock.off('message');
        this._sock.off('open');
    }

    sendCtrlAltDel() {
        if (this._tsdConnectionState !== 'connected' || this._viewOnly) { return; }
        Log.Info("Sending Ctrl-Alt-Del");

        this.sendKey(KeyTable.XK_Control_L, "ControlLeft", true);
        this.sendKey(KeyTable.XK_Alt_L, "AltLeft", true);
        this.sendKey(KeyTable.XK_Delete, "Delete", true);
        this.sendKey(KeyTable.XK_Delete, "Delete", false);
        this.sendKey(KeyTable.XK_Alt_L, "AltLeft", false);
        this.sendKey(KeyTable.XK_Control_L, "ControlLeft", false);
    }

    // Send a key press. If 'down' is not specified then send a down key
    // followed by an up key.
    sendKey(keysym, code, down) {
        if (this._tsdConnectionState !== 'connected' || this._viewOnly) { return; }

        if (down === undefined) {
            this.sendKey(keysym, code, true);
            this.sendKey(keysym, code, false);
            return;
        }

        const scancode = XtScancode[code];

        if (this._qemuExtKeyEventSupported && scancode) {
            // 0 is NoSymbol
            keysym = keysym || 0;

            Log.Info("Sending key (" + (down ? "down" : "up") + "): keysym " + keysym + ", scancode " + scancode);

            TSD.messages.extendedKeyEvent(this._sock, keysym, down, scancode);
        } else {
            if (!keysym) {
                return;
            }
            Log.Info("Sending keysym (" + (down ? "down" : "up") + "): " + keysym);
            TSD.messages.keyEvent(this._sock, keysym, down ? 1 : 0);
        }
    }

    focus() {
        this._canvas.focus();
    }

    blur() {
        this._canvas.blur();
    }

    clipboardPasteFrom(text) {
        if (this._tsdConnectionState !== 'connected' || this._viewOnly) { return; }

        if (this._clipboardServerCapabilitiesFormats[extendedClipboardFormatText] &&
            this._clipboardServerCapabilitiesActions[extendedClipboardActionNotify]) {

            this._clipboardText = text;
            TSD.messages.extendedClipboardNotify(this._sock, [extendedClipboardFormatText]);
        } else {
            let data = new Uint8Array(text.length);
            for (let i = 0; i < text.length; i++) {
                // FIXME: text can have values outside of Latin1/Uint8
                data[i] = text.charCodeAt(i);
            }

            TSD.messages.clientCutText(this._sock, data);
        }
    }

    // ===== PRIVATE METHODS =====

    _connect() {
        Log.Debug(">> TSD.connect");

        Log.Info("connecting to " + this._url);

        try {
            // WebSocket.onopen transitions to the TSD init states
            this._sock.open(this._url, this._wsProtocols);
        } catch (e) {
            if (e.name === 'SyntaxError') {
                this._fail("Invalid host or port (" + e + ")");
            } else {
                this._fail("Error when opening socket (" + e + ")");
            }
        }

        // Make our elements part of the page
        this._target.appendChild(this._screen);

        this._gestures.attach(this._canvas);

        this._cursor.attach(this._canvas);
        this._refreshCursor();

        // Monitor size changes of the screen
        // FIXME: Use ResizeObserver, or hidden overflow
        window.addEventListener('resize', this._eventHandlers.windowResize);

        // Always grab focus on some kind of click event
        this._canvas.addEventListener("mousedown", this._eventHandlers.focusCanvas);
        this._canvas.addEventListener("touchstart", this._eventHandlers.focusCanvas);

        // Mouse events
        this._canvas.addEventListener('mousedown', this._eventHandlers.handleMouse);
        this._canvas.addEventListener('mouseup', this._eventHandlers.handleMouse);
        this._canvas.addEventListener('mousemove', this._eventHandlers.handleMouse);
        // Prevent middle-click pasting (see handler for why we bind to document)
        this._canvas.addEventListener('click', this._eventHandlers.handleMouse);
        // preventDefault() on mousedown doesn't stop this event for some
        // reason so we have to explicitly block it
        this._canvas.addEventListener('contextmenu', this._eventHandlers.handleMouse);

        // Wheel events
        this._canvas.addEventListener("wheel", this._eventHandlers.handleWheel);

        // Gesture events
        this._canvas.addEventListener("gesturestart", this._eventHandlers.handleGesture);
        this._canvas.addEventListener("gesturemove", this._eventHandlers.handleGesture);
        this._canvas.addEventListener("gestureend", this._eventHandlers.handleGesture);

        Log.Debug("<< TSD.connect");
    }

    _disconnect() {
        Log.Debug(">> TSD.disconnect");
        this._cursor.detach();
        this._canvas.removeEventListener("gesturestart", this._eventHandlers.handleGesture);
        this._canvas.removeEventListener("gesturemove", this._eventHandlers.handleGesture);
        this._canvas.removeEventListener("gestureend", this._eventHandlers.handleGesture);
        this._canvas.removeEventListener("wheel", this._eventHandlers.handleWheel);
        this._canvas.removeEventListener('mousedown', this._eventHandlers.handleMouse);
        this._canvas.removeEventListener('mouseup', this._eventHandlers.handleMouse);
        this._canvas.removeEventListener('mousemove', this._eventHandlers.handleMouse);
        this._canvas.removeEventListener('click', this._eventHandlers.handleMouse);
        this._canvas.removeEventListener('contextmenu', this._eventHandlers.handleMouse);
        this._canvas.removeEventListener("mousedown", this._eventHandlers.focusCanvas);
        this._canvas.removeEventListener("touchstart", this._eventHandlers.focusCanvas);
        window.removeEventListener('resize', this._eventHandlers.windowResize);
        this._keyboard.ungrab();
        this._gestures.detach();
        this._sock.close();
        try {
            this._target.removeChild(this._screen);
        } catch (e) {
            if (e.name === 'NotFoundError') {
                // Some cases where the initial connection fails
                // can disconnect before the _screen is created
            } else {
                throw e;
            }
        }
        clearTimeout(this._resizeTimeout);
        clearTimeout(this._mouseMoveTimer);
        Log.Debug("<< TSD.disconnect");
    }

    _focusCanvas(event) {
        if (!this.focusOnClick) {
            return;
        }

        this.focus();
    }

    _setDesktopName(name) {
        this._fbName = name;
        this.dispatchEvent(new CustomEvent(
            "desktopname",
            { detail: { name: this._fbName } }));
    }

    _windowResize(event) {
        // If the window resized then our screen element might have
        // as well. Update the viewport dimensions.
        window.requestAnimationFrame(() => {
            this._updateClip();
            this._updateScale();
        });
    }

    // Update state of clipping in Display object, and make sure the
    // configured viewport matches the current screen size
    _updateClip() {
        const curClip = this._display.clipViewport;
        let newClip = this._clipViewport;

        if (this._scaleViewport) {
            // Disable viewport clipping if we are scaling
            newClip = false;
        }

        if (curClip !== newClip) {
            this._display.clipViewport = newClip;
            this._display_webrtc.clipViewport = newClip;
        }

        if (newClip) {
            // When clipping is enabled, the screen is limited to
            // the size of the container.
            const size = this._screenSize();
            this._display.viewportChangeSize(size.w, size.h);
            this._display_webrtc.viewportChangeSize(size.w, size.h);
            this._fixScrollbars();
        }
    }

    _updateScale() {
        if (!this._scaleViewport) {
            this._display.scale = 1.0;
            this._display_webrtc.scale = 1.0;
        } else {
            const size = this._screenSize();
            this._display.autoscale(size.w, size.h);
            this._display_webrtc.autoscale(size.w, size.h);
        }
        this._fixScrollbars();
    }

    // Gets the the size of the available screen
    _screenSize() {
        let r = this._screen.getBoundingClientRect();
        return { w: r.width, h: r.height };
    }

    _fixScrollbars() {
        // This is a hack because Chrome screws up the calculation
        // for when scrollbars are needed. So to fix it we temporarily
        // toggle them off and on.
        const orig = this._screen.style.overflow;
        this._screen.style.overflow = 'hidden';
        // Force Chrome to recalculate the layout by asking for
        // an element's dimensions
        this._screen.getBoundingClientRect();
        this._screen.style.overflow = orig;
    }

    /*
     * Connection states:
     *   connecting
     *   connected
     *   disconnecting
     *   disconnected - permanent state
     */
    _updateConnectionState(state) {
        const oldstate = this._tsdConnectionState;

        if (state === oldstate) {
            Log.Debug("Already in state '" + state + "', ignoring");
            return;
        }

        // The 'disconnected' state is permanent for each TSD object
        if (oldstate === 'disconnected') {
            Log.Error("Tried changing state of a disconnected TSD object");
            return;
        }

        // Ensure proper transitions before doing anything
        switch (state) {
            case 'connected':
                if (oldstate !== 'connecting') {
                    Log.Error("Bad transition to connected state, " +
                               "previous connection state: " + oldstate);
                    return;
                }
                break;

            case 'disconnected':
                if (oldstate !== 'disconnecting') {
                    Log.Error("Bad transition to disconnected state, " +
                               "previous connection state: " + oldstate);
                    return;
                }
                break;

            case 'connecting':
                if (oldstate !== '') {
                    Log.Error("Bad transition to connecting state, " +
                               "previous connection state: " + oldstate);
                    return;
                }
                break;

            case 'disconnecting':
                if (oldstate !== 'connected' && oldstate !== 'connecting') {
                    Log.Error("Bad transition to disconnecting state, " +
                               "previous connection state: " + oldstate);
                    return;
                }
                break;

            default:
                Log.Error("Unknown connection state: " + state);
                return;
        }

        // State change actions

        this._tsdConnectionState = state;

        Log.Debug("New state '" + state + "', was '" + oldstate + "'.");

        if (this._disconnTimer && state !== 'disconnecting') {
            Log.Debug("Clearing disconnect timer");
            clearTimeout(this._disconnTimer);
            this._disconnTimer = null;

            // make sure we don't get a double event
            this._sock.off('close');
        }

        switch (state) {
            case 'connecting':
                this._connect();
                break;

            case 'connected':
                this.dispatchEvent(new CustomEvent("connect", { detail: {} }));
                break;

            case 'disconnecting':
                this._disconnect();

                this._disconnTimer = setTimeout(() => {
                    Log.Error("Disconnection timed out.");
                    this._updateConnectionState('disconnected');
                }, DISCONNECT_TIMEOUT * 1000);
                break;

            case 'disconnected':
                this.dispatchEvent(new CustomEvent(
                    "disconnect", { detail:
                                    { clean: this._tsdCleanDisconnect } }));
                break;
        }
    }

    /* Print errors and disconnect
     *
     * The parameter 'details' is used for information that
     * should be logged but not sent to the user interface.
     */
    _fail(details) {
        switch (this._tsdConnectionState) {
            case 'disconnecting':
                Log.Error("Failed when disconnecting: " + details);
                break;
            case 'connected':
                Log.Error("Failed while connected: " + details);
                break;
            case 'connecting':
                Log.Error("Failed when connecting: " + details);
                break;
            default:
                Log.Error("TSD failure: " + details);
                break;
        }
        this._tsdCleanDisconnect = false; //This is sent to the UI

        // Transition to disconnected without waiting for socket to close
        this._updateConnectionState('disconnecting');
        this._updateConnectionState('disconnected');

        return false;
    }

    _setCapability(cap, val) {
        this._capabilities[cap] = val;
        this.dispatchEvent(new CustomEvent("capabilities",
                                           { detail: { capabilities: this._capabilities } }));
    }

    _handleMessage() {
        if (this._sock.rQlen === 0) {
            Log.Warn("handleMessage called on an empty receive queue");
            return;
        }

        switch (this._tsdConnectionState) {
            case 'disconnected':
                Log.Error("Got data while disconnected");
                break;
            case 'connected':
                while (true) {
                    if (this._flushing) {
                        break;
                    }
                    if (!this._normalMsg()) {
                        break;
                    }
                    if (this._sock.rQlen === 0) {
                        break;
                    }
                }
                break;
            default:
                this._initMsg();
                break;
        }
    }

    _handleKeyEvent(keysym, code, down) {
        this.sendKey(keysym, code, down);
    }

    _handleMouse(ev) {
        /*
         * We don't check connection status or viewOnly here as the
         * mouse events might be used to control the viewport
         */

        if (ev.type === 'click') {
            /*
             * Note: This is only needed for the 'click' event as it fails
             *       to fire properly for the target element so we have
             *       to listen on the document element instead.
             */
            if (ev.target !== this._canvas) {
                return;
            }
        }

        // FIXME: if we're in view-only and not dragging,
        //        should we stop events?
        ev.stopPropagation();
        ev.preventDefault();

        if ((ev.type === 'click') || (ev.type === 'contextmenu')) {
            return;
        }

        let pos = clientToElement(ev.clientX, ev.clientY,
                                  this._canvas);

        switch (ev.type) {
            case 'mousedown':
                setCapture(this._canvas);
                this._handleMouseButton(pos.x, pos.y,
                                        true, 1 << ev.button);
                break;
            case 'mouseup':
                this._handleMouseButton(pos.x, pos.y,
                                        false, 1 << ev.button);
                break;
            case 'mousemove':
                this._handleMouseMove(pos.x, pos.y);
                break;
        }
    }

    _handleMouseButton(x, y, down, bmask) {
        if (this.dragViewport) {
            if (down && !this._viewportDragging) {
                this._viewportDragging = true;
                this._viewportDragPos = {'x': x, 'y': y};
                this._viewportHasMoved = false;

                // Skip sending mouse events
                return;
            } else {
                this._viewportDragging = false;

                // If we actually performed a drag then we are done
                // here and should not send any mouse events
                if (this._viewportHasMoved) {
                    return;
                }

                // Otherwise we treat this as a mouse click event.
                // Send the button down event here, as the button up
                // event is sent at the end of this function.
                this._sendMouse(x, y, bmask);
            }
        }

        // Flush waiting move event first
        if (this._mouseMoveTimer !== null) {
            clearTimeout(this._mouseMoveTimer);
            this._mouseMoveTimer = null;
            this._sendMouse(x, y, this._mouseButtonMask);
        }

        if (down) {
            this._mouseButtonMask |= bmask;
        } else {
            this._mouseButtonMask &= ~bmask;
        }

        this._sendMouse(x, y, this._mouseButtonMask);
    }

    _handleMouseMove(x, y) {
        if (this._viewportDragging) {
            const deltaX = this._viewportDragPos.x - x;
            const deltaY = this._viewportDragPos.y - y;

            if (this._viewportHasMoved || (Math.abs(deltaX) > dragThreshold ||
                                           Math.abs(deltaY) > dragThreshold)) {
                this._viewportHasMoved = true;

                this._viewportDragPos = {'x': x, 'y': y};
                this._display.viewportChangePos(deltaX, deltaY);
            }

            // Skip sending mouse events
            return;
        }

        this._mousePos = { 'x': x, 'y': y };

        // Limit many mouse move events to one every MOUSE_MOVE_DELAY ms
        if (this._mouseMoveTimer == null) {

            const timeSinceLastMove = Date.now() - this._mouseLastMoveTime;
            if (timeSinceLastMove > MOUSE_MOVE_DELAY) {
                this._sendMouse(x, y, this._mouseButtonMask);
                this._mouseLastMoveTime = Date.now();
            } else {
                // Too soon since the latest move, wait the remaining time
                this._mouseMoveTimer = setTimeout(() => {
                    this._handleDelayedMouseMove();
                }, MOUSE_MOVE_DELAY - timeSinceLastMove);
            }
        }
    }

    _handleDelayedMouseMove() {
        this._mouseMoveTimer = null;
        this._sendMouse(this._mousePos.x, this._mousePos.y,
                        this._mouseButtonMask);
        this._mouseLastMoveTime = Date.now();
    }

    _sendMouse(x, y, mask) {
        if (this._tsdConnectionState !== 'connected') { return; }
        if (this._viewOnly) { return; } // View only, skip mouse events

        TSD.messages.pointerEvent(this._sock, this._display.absX(x),
                                  this._display.absY(y), mask);
    }

    _handleWheel(ev) {
        if (this._tsdConnectionState !== 'connected') { return; }
        if (this._viewOnly) { return; } // View only, skip mouse events

        ev.stopPropagation();
        ev.preventDefault();

        let pos = clientToElement(ev.clientX, ev.clientY,
                                  this._canvas);

        let dX = ev.deltaX;
        let dY = ev.deltaY;

        // Pixel units unless it's non-zero.
        // Note that if deltamode is line or page won't matter since we aren't
        // sending the mouse wheel delta to the server anyway.
        // The difference between pixel and line can be important however since
        // we have a threshold that can be smaller than the line height.
        if (ev.deltaMode !== 0) {
            dX *= WHEEL_LINE_HEIGHT;
            dY *= WHEEL_LINE_HEIGHT;
        }

        // Mouse wheel events are sent in steps over VNC. This means that the VNC
        // protocol can't handle a wheel event with specific distance or speed.
        // Therefor, if we get a lot of small mouse wheel events we combine them.
        this._accumulatedWheelDeltaX += dX;
        this._accumulatedWheelDeltaY += dY;

        // Generate a mouse wheel step event when the accumulated delta
        // for one of the axes is large enough.
        if (Math.abs(this._accumulatedWheelDeltaX) >= WHEEL_STEP) {
            if (this._accumulatedWheelDeltaX < 0) {
                this._handleMouseButton(pos.x, pos.y, true, 1 << 5);
                this._handleMouseButton(pos.x, pos.y, false, 1 << 5);
            } else if (this._accumulatedWheelDeltaX > 0) {
                this._handleMouseButton(pos.x, pos.y, true, 1 << 6);
                this._handleMouseButton(pos.x, pos.y, false, 1 << 6);
            }

            this._accumulatedWheelDeltaX = 0;
        }
        if (Math.abs(this._accumulatedWheelDeltaY) >= WHEEL_STEP) {
            if (this._accumulatedWheelDeltaY < 0) {
                this._handleMouseButton(pos.x, pos.y, true, 1 << 3);
                this._handleMouseButton(pos.x, pos.y, false, 1 << 3);
            } else if (this._accumulatedWheelDeltaY > 0) {
                this._handleMouseButton(pos.x, pos.y, true, 1 << 4);
                this._handleMouseButton(pos.x, pos.y, false, 1 << 4);
            }

            this._accumulatedWheelDeltaY = 0;
        }
    }

    _fakeMouseMove(ev, elementX, elementY) {
        this._handleMouseMove(elementX, elementY);
        this._cursor.move(ev.detail.clientX, ev.detail.clientY);
    }

    _handleTapEvent(ev, bmask) {
        let pos = clientToElement(ev.detail.clientX, ev.detail.clientY,
                                  this._canvas);

        // If the user quickly taps multiple times we assume they meant to
        // hit the same spot, so slightly adjust coordinates

        if ((this._gestureLastTapTime !== null) &&
            ((Date.now() - this._gestureLastTapTime) < DOUBLE_TAP_TIMEOUT) &&
            (this._gestureFirstDoubleTapEv.detail.type === ev.detail.type)) {
            let dx = this._gestureFirstDoubleTapEv.detail.clientX - ev.detail.clientX;
            let dy = this._gestureFirstDoubleTapEv.detail.clientY - ev.detail.clientY;
            let distance = Math.hypot(dx, dy);

            if (distance < DOUBLE_TAP_THRESHOLD) {
                pos = clientToElement(this._gestureFirstDoubleTapEv.detail.clientX,
                                      this._gestureFirstDoubleTapEv.detail.clientY,
                                      this._canvas);
            } else {
                this._gestureFirstDoubleTapEv = ev;
            }
        } else {
            this._gestureFirstDoubleTapEv = ev;
        }
        this._gestureLastTapTime = Date.now();

        this._fakeMouseMove(this._gestureFirstDoubleTapEv, pos.x, pos.y);
        this._handleMouseButton(pos.x, pos.y, true, bmask);
        this._handleMouseButton(pos.x, pos.y, false, bmask);
    }

    _handleGesture(ev) {
        let magnitude;

        let pos = clientToElement(ev.detail.clientX, ev.detail.clientY,
                                  this._canvas);
        switch (ev.type) {
            case 'gesturestart':
                switch (ev.detail.type) {
                    case 'onetap':
                        this._handleTapEvent(ev, 0x1);
                        break;
                    case 'twotap':
                        this._handleTapEvent(ev, 0x4);
                        break;
                    case 'threetap':
                        this._handleTapEvent(ev, 0x2);
                        break;
                    case 'drag':
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        this._handleMouseButton(pos.x, pos.y, true, 0x1);
                        break;
                    case 'longpress':
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        this._handleMouseButton(pos.x, pos.y, true, 0x4);
                        break;

                    case 'twodrag':
                        this._gestureLastMagnitudeX = ev.detail.magnitudeX;
                        this._gestureLastMagnitudeY = ev.detail.magnitudeY;
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        break;
                    case 'pinch':
                        this._gestureLastMagnitudeX = Math.hypot(ev.detail.magnitudeX,
                                                                 ev.detail.magnitudeY);
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        break;
                }
                break;

            case 'gesturemove':
                switch (ev.detail.type) {
                    case 'onetap':
                    case 'twotap':
                    case 'threetap':
                        break;
                    case 'drag':
                    case 'longpress':
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        break;
                    case 'twodrag':
                        // Always scroll in the same position.
                        // We don't know if the mouse was moved so we need to move it
                        // every update.
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        while ((ev.detail.magnitudeY - this._gestureLastMagnitudeY) > GESTURE_SCRLSENS) {
                            this._handleMouseButton(pos.x, pos.y, true, 0x8);
                            this._handleMouseButton(pos.x, pos.y, false, 0x8);
                            this._gestureLastMagnitudeY += GESTURE_SCRLSENS;
                        }
                        while ((ev.detail.magnitudeY - this._gestureLastMagnitudeY) < -GESTURE_SCRLSENS) {
                            this._handleMouseButton(pos.x, pos.y, true, 0x10);
                            this._handleMouseButton(pos.x, pos.y, false, 0x10);
                            this._gestureLastMagnitudeY -= GESTURE_SCRLSENS;
                        }
                        while ((ev.detail.magnitudeX - this._gestureLastMagnitudeX) > GESTURE_SCRLSENS) {
                            this._handleMouseButton(pos.x, pos.y, true, 0x20);
                            this._handleMouseButton(pos.x, pos.y, false, 0x20);
                            this._gestureLastMagnitudeX += GESTURE_SCRLSENS;
                        }
                        while ((ev.detail.magnitudeX - this._gestureLastMagnitudeX) < -GESTURE_SCRLSENS) {
                            this._handleMouseButton(pos.x, pos.y, true, 0x40);
                            this._handleMouseButton(pos.x, pos.y, false, 0x40);
                            this._gestureLastMagnitudeX -= GESTURE_SCRLSENS;
                        }
                        break;
                    case 'pinch':
                        // Always scroll in the same position.
                        // We don't know if the mouse was moved so we need to move it
                        // every update.
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        magnitude = Math.hypot(ev.detail.magnitudeX, ev.detail.magnitudeY);
                        if (Math.abs(magnitude - this._gestureLastMagnitudeX) > GESTURE_ZOOMSENS) {
                            this._handleKeyEvent(KeyTable.XK_Control_L, "ControlLeft", true);
                            while ((magnitude - this._gestureLastMagnitudeX) > GESTURE_ZOOMSENS) {
                                this._handleMouseButton(pos.x, pos.y, true, 0x8);
                                this._handleMouseButton(pos.x, pos.y, false, 0x8);
                                this._gestureLastMagnitudeX += GESTURE_ZOOMSENS;
                            }
                            while ((magnitude -  this._gestureLastMagnitudeX) < -GESTURE_ZOOMSENS) {
                                this._handleMouseButton(pos.x, pos.y, true, 0x10);
                                this._handleMouseButton(pos.x, pos.y, false, 0x10);
                                this._gestureLastMagnitudeX -= GESTURE_ZOOMSENS;
                            }
                        }
                        this._handleKeyEvent(KeyTable.XK_Control_L, "ControlLeft", false);
                        break;
                }
                break;

            case 'gestureend':
                switch (ev.detail.type) {
                    case 'onetap':
                    case 'twotap':
                    case 'threetap':
                    case 'pinch':
                    case 'twodrag':
                        break;
                    case 'drag':
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        this._handleMouseButton(pos.x, pos.y, false, 0x1);
                        break;
                    case 'longpress':
                        this._fakeMouseMove(ev, pos.x, pos.y);
                        this._handleMouseButton(pos.x, pos.y, false, 0x4);
                        break;
                }
                break;
        }
    }

    // Message Handlers

    _updateCursor(rgba, hotx, hoty, w, h) {
        this._cursorImage = {
            rgbaPixels: rgba,
            hotx: hotx, hoty: hoty, w: w, h: h,
        };
        this._refreshCursor();
    }

    _shouldShowDotCursor() {
        // Called when this._cursorImage is updated
        if (!this._showDotCursor) {
            // User does not want to see the dot, so...
            return false;
        }

        // The dot should not be shown if the cursor is already visible,
        // i.e. contains at least one not-fully-transparent pixel.
        // So iterate through all alpha bytes in rgba and stop at the
        // first non-zero.
        for (let i = 3; i < this._cursorImage.rgbaPixels.length; i += 4) {
            if (this._cursorImage.rgbaPixels[i]) {
                return false;
            }
        }

        // At this point, we know that the cursor is fully transparent, and
        // the user wants to see the dot instead of this.
        return true;
    }

    _refreshCursor() {
        if (this._tsdConnectionState !== "connecting" &&
            this._tsdConnectionState !== "connected") {
            return;
        }
        const image = this._shouldShowDotCursor() ? TSD.cursors.dot : this._cursorImage;
        this._cursor.change(image.rgbaPixels,
                            image.hotx, image.hoty,
                            image.w, image.h
        );
    }
}

// Class Methods
TSD.messages = {
    keyEvent(sock, keysym, down) {
        const buff = sock._sQ;
        const offset = sock._sQlen;

        buff[offset] = 4;  // msg-type
        buff[offset + 1] = down;

        buff[offset + 2] = 0;
        buff[offset + 3] = 0;

        buff[offset + 4] = (keysym >> 24);
        buff[offset + 5] = (keysym >> 16);
        buff[offset + 6] = (keysym >> 8);
        buff[offset + 7] = keysym;

        sock._sQlen += 8;
        sock.flush();
    },

    extendedKeyEvent(sock, keysym, down, keycode) {
        function getRFBkeycode(xtScanCode) {
            const upperByte = (keycode >> 8);
            const lowerByte = (keycode & 0x00ff);
            if (upperByte === 0xe0 && lowerByte < 0x7f) {
                return lowerByte | 0x80;
            }
            return xtScanCode;
        }

        const buff = sock._sQ;
        const offset = sock._sQlen;

        buff[offset] = 255; // msg-type
        buff[offset + 1] = 0; // sub msg-type

        buff[offset + 2] = (down >> 8);
        buff[offset + 3] = down;

        buff[offset + 4] = (keysym >> 24);
        buff[offset + 5] = (keysym >> 16);
        buff[offset + 6] = (keysym >> 8);
        buff[offset + 7] = keysym;

        const RFBkeycode = getRFBkeycode(keycode);

        buff[offset + 8] = (RFBkeycode >> 24);
        buff[offset + 9] = (RFBkeycode >> 16);
        buff[offset + 10] = (RFBkeycode >> 8);
        buff[offset + 11] = RFBkeycode;

        sock._sQlen += 12;
        sock.flush();
    },

    pointerEvent(sock, x, y, mask) {
        const buff = sock._sQ;
        const offset = sock._sQlen;

        buff[offset] = 5; // msg-type

        buff[offset + 1] = mask;

        buff[offset + 2] = x >> 8;
        buff[offset + 3] = x;

        buff[offset + 4] = y >> 8;
        buff[offset + 5] = y;

        sock._sQlen += 6;
        sock.flush();
    },

    clientCutText(sock, data, extended = false) {
        const buff = sock._sQ;
        const offset = sock._sQlen;

        buff[offset] = 6; // msg-type

        buff[offset + 1] = 0; // padding
        buff[offset + 2] = 0; // padding
        buff[offset + 3] = 0; // padding

        let length;
        if (extended) {
            length = toUnsigned32bit(-data.length);
        } else {
            length = data.length;
        }

        buff[offset + 4] = length >> 24;
        buff[offset + 5] = length >> 16;
        buff[offset + 6] = length >> 8;
        buff[offset + 7] = length;

        sock._sQlen += 8;

        // We have to keep track of from where in the data we begin creating the
        // buffer for the flush in the next iteration.
        let dataOffset = 0;

        let remaining = data.length;
        while (remaining > 0) {

            let flushSize = Math.min(remaining, (sock._sQbufferSize - sock._sQlen));
            for (let i = 0; i < flushSize; i++) {
                buff[sock._sQlen + i] = data[dataOffset + i];
            }

            sock._sQlen += flushSize;
            sock.flush();

            remaining -= flushSize;
            dataOffset += flushSize;
        }

    }
};

TSD.cursors = {
    none: {
        rgbaPixels: new Uint8Array(),
        w: 0, h: 0,
        hotx: 0, hoty: 0,
    },

    dot: {
        /* eslint-disable indent */
        rgbaPixels: new Uint8Array([
            255, 255, 255, 255,   0,   0,   0, 255, 255, 255, 255, 255,
              0,   0,   0, 255,   0,   0,   0,   0,   0,   0,  0,  255,
            255, 255, 255, 255,   0,   0,   0, 255, 255, 255, 255, 255,
        ]),
        /* eslint-enable indent */
        w: 3, h: 3,
        hotx: 1, hoty: 1,
    }
};
