package direct

/*
#cgo LDFLAGS: -framework CoreFoundation -framework CoreGraphics -framework CoreServices -framework IOKit -framework IOSurface -framework Carbon

#define XK_BackSpace		0xFF08	// back space, back char
#define XK_Tab			0xFF09
#define XK_Return		0xFF0D	// Return, enter
#define XK_Pause		0xFF13	// Pause, hold
#define XK_Scroll_Lock		0xFF14
#define XK_Escape		0xFF1B
#define XK_Delete		0xFFFF	// Delete, rubout

// Cursor control & motion

#define XK_Home			0xFF50
#define XK_Left			0xFF51	// Move left, left arrow
#define XK_Up			0xFF52	// Move up, up arrow
#define XK_Right		0xFF53	// Move right, right arrow
#define XK_Down			0xFF54	// Move down, down arrow
#define XK_Prior		0xFF55	// Prior, previous
#define XK_Page_Up		0xFF55
#define XK_Next			0xFF56	// Next
#define XK_Page_Down	0xFF56
#define XK_End			0xFF57	// EOL
#define XK_Begin		0xFF58	// BOL


// Misc Functions

#define XK_Insert		0xFF63	// Insert, insert here
#define XK_Num_Lock		0xFF7F

// Keypad Functions, keypad numbers cleverly chosen to map to ascii

#define XK_KP_Enter		0xFF8D	// enter
#define XK_KP_Equal		0xFFBD	// equals
#define XK_KP_Multiply		0xFFAA
#define XK_KP_Add		0xFFAB
#define XK_KP_Subtract		0xFFAD
#define XK_KP_Decimal		0xFFAE
#define XK_KP_Divide		0xFFAF

#define XK_KP_0			0xFFB0
#define XK_KP_1			0xFFB1
#define XK_KP_2			0xFFB2
#define XK_KP_3			0xFFB3
#define XK_KP_4			0xFFB4
#define XK_KP_5			0xFFB5
#define XK_KP_6			0xFFB6
#define XK_KP_7			0xFFB7
#define XK_KP_8			0xFFB8
#define XK_KP_9			0xFFB9

#define XK_F1			0xFFBE
#define XK_F2			0xFFBF
#define XK_F3			0xFFC0
#define XK_F4			0xFFC1
#define XK_F5			0xFFC2
#define XK_F6			0xFFC3
#define XK_F7			0xFFC4
#define XK_F8			0xFFC5
#define XK_F9			0xFFC6
#define XK_F10			0xFFC7
#define XK_F11			0xFFC8
#define XK_F12			0xFFC9

// Modifiers

#define XK_Shift_L		0xFFE1	// Left shift
#define XK_Shift_R		0xFFE2	// Right shift
#define XK_Control_L		0xFFE3	// Left control
#define XK_Control_R		0xFFE4	// Right control
#define XK_Caps_Lock		0xFFE5	// Caps lock
#define XK_Shift_Lock		0xFFE6	// Shift lock

#define XK_Meta_L		0xFFE7	// Left meta
#define XK_Meta_R		0xFFE8	// Right meta
#define XK_Alt_L		0xFFE9	// Left alt
#define XK_Alt_R		0xFFEA	// Right alt
#define XK_Super_L		0xFFEB	// Left super
#define XK_Super_R		0xFFEC	// Right super
#define XK_Hyper_L		0xFFED	// Left hyper
#define XK_Hyper_R		0xFFEE	// Right hyper // XK_MISCELLANY

// ISO 9995 Function and Modifier Keys
#define	XK_ISO_Level3_Shift				0xFE03

#define XK_space               0x020

#include <Carbon/Carbon.h>
#include <IOSurface/IOSurface.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <IOKit/pwr_mgt/IOPM.h>
#include <stdio.h>
#include <stdbool.h>

// The corresponding multi-sceen display ID
CGDirectDisplayID displayID;

// The server's private event source
CGEventSourceRef eventSource;

// a dictionary mapping characters to keycodes
CFMutableDictionaryRef charKeyMap;

// a dictionary mapping characters obtained by Shift to keycodes
CFMutableDictionaryRef charShiftKeyMap;

// a dictionary mapping characters obtained by Alt-Gr to keycodes
CFMutableDictionaryRef charAltGrKeyMap;

// a dictionary mapping characters obtained by Shift+Alt-Gr to keycodes
CFMutableDictionaryRef charShiftAltGrKeyMap;

// a table mapping special keys to keycodes. static as these are layout-independent
static int specialKeyMap[] = {
    // "Special" keys
    XK_space,             49,      // Space
    XK_Return,            36,      // Return
    XK_Delete,           117,      // Delete
    XK_Tab,               48,      // Tab
    XK_Escape,            53,      // Esc
    XK_Caps_Lock,         57,      // Caps Lock
    XK_Num_Lock,          71,      // Num Lock
    XK_Scroll_Lock,      107,      // Scroll Lock
    XK_Pause,            113,      // Pause
    XK_BackSpace,         51,      // Backspace
    XK_Insert,           114,      // Insert

    // Cursor movement
    XK_Up,               126,      // Cursor Up
    XK_Down,             125,      // Cursor Down
    XK_Left,             123,      // Cursor Left
    XK_Right,            124,      // Cursor Right
    XK_Page_Up,          116,      // Page Up
    XK_Page_Down,        121,      // Page Down
    XK_Home,             115,      // Home
    XK_End,              119,      // End

    // Numeric keypad
    XK_KP_0,              82,      // KP 0
    XK_KP_1,              83,      // KP 1
    XK_KP_2,              84,      // KP 2
    XK_KP_3,              85,      // KP 3
    XK_KP_4,              86,      // KP 4
    XK_KP_5,              87,      // KP 5
    XK_KP_6,              88,      // KP 6
    XK_KP_7,              89,      // KP 7
    XK_KP_8,              91,      // KP 8
    XK_KP_9,              92,      // KP 9
    XK_KP_Enter,          76,      // KP Enter
    XK_KP_Decimal,        65,      // KP .
    XK_KP_Add,            69,      // KP +
    XK_KP_Subtract,       78,      // KP -
    XK_KP_Multiply,       67,      // KP *
    XK_KP_Divide,         75,      // KP /

    // Function keys
    XK_F1,               122,      // F1
    XK_F2,               120,      // F2
    XK_F3,                99,      // F3
    XK_F4,               118,      // F4
    XK_F5,                96,      // F5
    XK_F6,                97,      // F6
    XK_F7,                98,      // F7
    XK_F8,               100,      // F8
    XK_F9,               101,      // F9
    XK_F10,              109,      // F10
    XK_F11,              103,      // F11
    XK_F12,              111,      // F12

    // Modifier keys
    XK_Shift_L,           56,      // Shift Left
    XK_Shift_R,           56,      // Shift Right
    XK_Control_L,         59,      // Ctrl Left
    XK_Control_R,         59,      // Ctrl Right
    XK_Meta_L,            58,      // Logo Left (-> Option)
    XK_Meta_R,            58,      // Logo Right (-> Option)
    XK_Alt_L,             55,      // Alt Left (-> Command)
    XK_Alt_R,             55,      // Alt Right (-> Command)
    XK_ISO_Level3_Shift,  61,      // Alt-Gr (-> Option Right)
    0x1008FF2B,           63,      // Fn
};

// Global shifting modifier states
bool isShiftDown;
bool isAltGrDown;

bool ScreenInit() {
	printf("Using primary display as a default\n");
	displayID = CGMainDisplayID();
	return TRUE;
}

bool KeyboardInit() {
    size_t i, keyCodeCount=128;
    TISInputSourceRef currentKeyboard = TISCopyCurrentKeyboardInputSource();
    const UCKeyboardLayout *keyboardLayout;

    if(!currentKeyboard) {
		fprintf(stderr, "Could not get current keyboard info\n");
		return FALSE;
    }

    keyboardLayout = (const UCKeyboardLayout *)CFDataGetBytePtr(TISGetInputSourceProperty(currentKeyboard, kTISPropertyUnicodeKeyLayoutData));

    printf("Found keyboard layout '%s'\n", CFStringGetCStringPtr(TISGetInputSourceProperty(currentKeyboard, kTISPropertyInputSourceID), kCFStringEncodingUTF8));

    charKeyMap = CFDictionaryCreateMutable(kCFAllocatorDefault, keyCodeCount, &kCFCopyStringDictionaryKeyCallBacks, NULL);
    charShiftKeyMap = CFDictionaryCreateMutable(kCFAllocatorDefault, keyCodeCount, &kCFCopyStringDictionaryKeyCallBacks, NULL);
    charAltGrKeyMap = CFDictionaryCreateMutable(kCFAllocatorDefault, keyCodeCount, &kCFCopyStringDictionaryKeyCallBacks, NULL);
    charShiftAltGrKeyMap = CFDictionaryCreateMutable(kCFAllocatorDefault, keyCodeCount, &kCFCopyStringDictionaryKeyCallBacks, NULL);

    if(!charKeyMap || !charShiftKeyMap || !charAltGrKeyMap || !charShiftAltGrKeyMap) {
		fprintf(stderr, "Could not create keymaps\n");
		return FALSE;
    }

    // Loop through every keycode to find the character it is mapping to.
    for (i = 0; i < keyCodeCount; ++i) {
	UInt32 deadKeyState = 0;
	UniChar chars[4];
	UniCharCount realLength;
	UInt32 m, modifiers[] = {0, kCGEventFlagMaskShift, kCGEventFlagMaskAlternate, kCGEventFlagMaskShift|kCGEventFlagMaskAlternate};

	// do this for no modifier, shift and alt-gr applied
	for(m = 0; m < sizeof(modifiers) / sizeof(modifiers[0]); ++m) {
	    UCKeyTranslate(keyboardLayout,
			   i,
			   kUCKeyActionDisplay,
			   (modifiers[m] >> 16) & 0xff,
			   LMGetKbdType(),
			   kUCKeyTranslateNoDeadKeysBit,
			   &deadKeyState,
			   sizeof(chars) / sizeof(chars[0]),
			   &realLength,
			   chars);

	    CFStringRef string = CFStringCreateWithCharacters(kCFAllocatorDefault, chars, 1);
	    if(string) {
		switch(modifiers[m]) {
		case 0:
		    CFDictionaryAddValue(charKeyMap, string, (const void *)i);
		    break;
		case kCGEventFlagMaskShift:
		    CFDictionaryAddValue(charShiftKeyMap, string, (const void *)i);
		    break;
		case kCGEventFlagMaskAlternate:
		    CFDictionaryAddValue(charAltGrKeyMap, string, (const void *)i);
		    break;
		case kCGEventFlagMaskShift|kCGEventFlagMaskAlternate:
		    CFDictionaryAddValue(charShiftAltGrKeyMap, string, (const void *)i);
		    break;
		}

		CFRelease(string);
	    }
	}
    }

    CFRelease(currentKeyboard);

    return TRUE;
}

void KbdAddEvent(uint32_t keySym, bool down) {
    int i;
    CGKeyCode keyCode = -1;
    CGEventRef keyboardEvent;
    int specialKeyFound = 0;

    // look for special key
    for (i = 0; i < (sizeof(specialKeyMap) / sizeof(int)); i += 2) {
        if (specialKeyMap[i] == keySym) {
            keyCode = specialKeyMap[i+1];
            specialKeyFound = 1;
            break;
        }
    }

    if(specialKeyFound) {
		// keycode for special key found
		keyboardEvent = CGEventCreateKeyboardEvent(eventSource, keyCode, down);
		// save state of shifting modifiers
		if(keySym == XK_ISO_Level3_Shift)
			isAltGrDown = down;
		if(keySym == XK_Shift_L || keySym == XK_Shift_R)
			isShiftDown = down;
    } else {
		// look for char key
		size_t keyCodeFromDict;
		CFStringRef charStr = CFStringCreateWithCharacters(kCFAllocatorDefault, (UniChar*)&keySym, 1);
		CFMutableDictionaryRef keyMap = charKeyMap;
		if(isShiftDown && !isAltGrDown)
			keyMap = charShiftKeyMap;
		if(!isShiftDown && isAltGrDown)
			keyMap = charAltGrKeyMap;
		if(isShiftDown && isAltGrDown)
			keyMap = charShiftAltGrKeyMap;

		if (CFDictionaryGetValueIfPresent(keyMap, charStr, (const void **)&keyCodeFromDict)) {
			// keycode for ASCII key found
			keyboardEvent = CGEventCreateKeyboardEvent(eventSource, keyCodeFromDict, down);
		} else {
			// last resort: use the symbol's utf-16 value, does not support modifiers though
			keyboardEvent = CGEventCreateKeyboardEvent(eventSource, 0, down);
			CGEventKeyboardSetUnicodeString(keyboardEvent, 1, (UniChar*)&keySym);
		}

		CFRelease(charStr);
    }

    // Set the Shift modifier explicitly as MacOS sometimes gets internal state wrong and Shift stuck.
    CGEventSetFlags(keyboardEvent, CGEventGetFlags(keyboardEvent) & (isShiftDown ? kCGEventFlagMaskShift : ~kCGEventFlagMaskShift));

    CGEventPost(kCGSessionEventTap, keyboardEvent);
    CFRelease(keyboardEvent);
}

void PtrAddEvent(int buttonMask, int x, int y) {
    CGPoint position;
    CGRect displayBounds = CGDisplayBounds(displayID);
    CGEventRef mouseEvent = NULL;

    position.x = x + displayBounds.origin.x;
    position.y = y + displayBounds.origin.y;

    // map buttons 4 5 6 7 to scroll events as per https://github.com/rfbproto/rfbproto/blob/master/rfbproto.rst#745pointerevent
    if(buttonMask & (1 << 3))
        mouseEvent = CGEventCreateScrollWheelEvent(eventSource, kCGScrollEventUnitLine, 2, 1, 0);
    if(buttonMask & (1 << 4))
        mouseEvent = CGEventCreateScrollWheelEvent(eventSource, kCGScrollEventUnitLine, 2, -1, 0);
    if(buttonMask & (1 << 5))
        mouseEvent = CGEventCreateScrollWheelEvent(eventSource, kCGScrollEventUnitLine, 2, 0, 1);
    if(buttonMask & (1 << 6))
        mouseEvent = CGEventCreateScrollWheelEvent(eventSource, kCGScrollEventUnitLine, 2, 0, -1);

    if (mouseEvent) {
        CGEventPost(kCGSessionEventTap, mouseEvent);
        CFRelease(mouseEvent);
    }
    else {
        CGPostMouseEvent(position, TRUE, 3,
                (buttonMask & (1 << 0)) ? TRUE : FALSE,
                (buttonMask & (1 << 2)) ? TRUE : FALSE,
                (buttonMask & (1 << 1)) ? TRUE : FALSE);
    }
}


*/
import "C"

import (
	"encoding/json"
	"os"

	"github.com/trevor403/gostream/pkg/input"
	"github.com/trevor403/gostream/pkg/worker"
)

func init() {
	if _, isChild := os.LookupEnv(worker.ChildEnvName); isChild {
		return
	}
	ret := C.KeyboardInit()
	if !bool(ret) {
		panic("keyboard init failed")
	}
	ret = C.ScreenInit()
	if !bool(ret) {
		panic("screen init failed")
	}
}

func Handle(data []byte) {
	raw := input.RawEvent{}
	_ = json.Unmarshal(data, &raw)

	switch raw.Type {
	case input.KeyEventType:
		ev := input.KeyEvent{}
		json.Unmarshal(data, &ev)
		HandleKey(ev)
	case input.MouseEventType:
		ev := input.MouseEvent{}
		json.Unmarshal(data, &ev)
		HandlePtr(ev)
	}
}

func HandlePtr(ev input.MouseEvent) error {
	// fmt.Println("sending ptr", ev)
	C.PtrAddEvent(C.int(ev.ButtonMask), C.int(ev.X), C.int(ev.Y))
	return nil
}

func HandleKey(ev input.KeyEvent) error {
	// fmt.Println("sending key", ev)
	C.KbdAddEvent(C.uint32(ev.Key), C.bool(ev.State > 0))
	return nil
}
