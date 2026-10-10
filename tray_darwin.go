package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// Wails' tray label is one line in the menu bar font; the item is drawn here instead, as one
// template image: the cat with a mode badge; the speed is the button's title.
static NSImage *trayIcon;

static void setTray(void *item, const void *icon, int iconLen, const char *badge, bool dim, const char *up, const char *down) {
	if (!trayIcon) {
		trayIcon = [[NSImage alloc] initWithData:[NSData dataWithBytes:icon length:iconLen]];
	}
	NSString *b = badge ? @(badge) : nil, *u = up ? @(up) : nil, *d = down ? @(down) : nil;
	dispatch_async(dispatch_get_main_queue(), ^{
		NSStatusBarButton *button = ((NSStatusItem *)item).button;
		CGFloat h = NSStatusBar.systemStatusBar.thickness, px = NSScreen.mainScreen.backingScaleFactor;
		CGFloat (^snap)(CGFloat) = ^CGFloat(CGFloat v) { return round(v * px) / px; };
		CGFloat catW = round(h * trayIcon.size.width / trayIcon.size.height);
		// the mode as a Surge-style badge: a rounded square at the cat's lower right, letter knocked out
		NSFont *bf = [NSFont systemFontOfSize:round(h * 0.39) * 0.75 weight:NSFontWeightHeavy];
		NSDictionary *ba = @{NSFontAttributeName: bf};
		CGFloat side = round(h * 0.39), iconW = b ? catW + round(h * 0.2) : catW; // Surge-sized
		NSRect box = NSMakeRect(iconW - side, round(h * 0.1), side, side);
		NSImage *img = [NSImage imageWithSize:NSMakeSize(iconW, h) flipped:NO drawingHandler:^BOOL(NSRect r) {
			CGContextRef cg = NSGraphicsContext.currentContext.CGContext;
			CGContextSetAlpha(cg, dim ? 0.4 : 1); // grey while nothing is captured
			CGContextBeginTransparencyLayer(cg, NULL);
			[trayIcon drawInRect:NSMakeRect(0, 0, catW, h)];
			if (b) {
				NSGraphicsContext *ctx = NSGraphicsContext.currentContext;
				ctx.compositingOperation = NSCompositingOperationClear; // a gap between badge and cat
				[[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(box, -1.2, -1.2) xRadius:3 yRadius:3] fill];
				ctx.compositingOperation = NSCompositingOperationSourceOver;
				[NSColor.blackColor set];
				[[NSBezierPath bezierPathWithRoundedRect:box xRadius:2 yRadius:2] fill];
				NSRect ink = [b boundingRectWithSize:NSZeroSize options:NSStringDrawingUsesDeviceMetrics attributes:ba];
				ctx.compositingOperation = NSCompositingOperationDestinationOut;
				// measured on screen: centring by ink bounds still leaves the letter 0.5pt high
				[b drawAtPoint:NSMakePoint(snap(NSMidX(box) - NSMidX(ink)), snap(NSMidY(box) - NSMidY(ink) - 0.5) + bf.descender) withAttributes:ba];
			}
			CGContextEndTransparencyLayer(cg);
			return YES;
		}];
		img.template = YES;
		button.imageScaling = NSImageScaleNone;
		button.image = img;
		button.imagePosition = NSImageLeft;
		if (!u) {
			button.title = @"";
			return;
		}
		// the speed is the button's own two-line title, so it renders like other menu bar text
		NSMutableParagraphStyle *p = [NSMutableParagraphStyle new];
		p.alignment = NSTextAlignmentRight;
		p.minimumLineHeight = p.maximumLineHeight = 9;
		button.attributedTitle = [[NSAttributedString alloc] initWithString:[NSString stringWithFormat:@"%@\n%@", u, d] attributes:@{
			NSFontAttributeName: [NSFont monospacedDigitSystemFontOfSize:9 weight:NSFontWeightMedium],
			NSParagraphStyleAttributeName: p,
			NSBaselineOffsetAttributeName: @(-4), // the button sits a two-line title high; centre it on the bar
		}];
	});
}
*/
import "C"

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// statusItem digs Wails' NSStatusItem out of the tray, which it doesn't expose; nil if a Wails
// upgrade moved it, or before the app has started.
func statusItem(tray *application.SystemTray) unsafe.Pointer {
	impl := reflect.ValueOf(tray).Elem().FieldByName("impl")
	if !impl.IsValid() || impl.IsNil() || impl.Elem().Kind() != reflect.Pointer {
		return nil
	}
	f := impl.Elem().Elem().FieldByName("nsStatusItem")
	if !f.IsValid() || f.Kind() != reflect.UnsafePointer {
		return nil
	}
	return unsafe.Pointer(f.Pointer())
}

// padRate left-pads a rate with figure spaces (digit-wide) to the length of "999 KB/s", which keeps
// the two lines right-aligned and the item's width steady.
func padRate(b int64) string {
	r := rate(b)
	return strings.Repeat("\u2007", max(0, 8-utf8.RuneCountInString(r))) + r
}

var modeBadges = map[string]string{"rule": "R", "global": "G", "direct": "D"}

// showTray draws the menu bar item: the cat badged with the outbound mode, greyed while nothing is
// captured (no system proxy, no enhanced mode), plus upload over download unless the speed is hidden.
func showTray(tray *application.SystemTray, t Traffic, mode string, capturing, showSpeed bool) {
	item := statusItem(tray)
	if item == nil {
		if showSpeed {
			tray.SetLabel(fmt.Sprintf("↑%s ↓%s", rate(t.Up), rate(t.Down)))
		}
		return
	}
	badge, up, down := (*C.char)(nil), (*C.char)(nil), (*C.char)(nil)
	if letter := modeBadges[mode]; letter != "" {
		badge = C.CString(letter)
	}
	if showSpeed {
		up, down = C.CString(padRate(t.Up)), C.CString(padRate(t.Down))
	}
	// the icon is copied on the first call only, before setTray returns
	C.setTray(item, unsafe.Pointer(&trayIcon[0]), C.int(len(trayIcon)), badge, C.bool(!capturing), up, down)
	for _, p := range []unsafe.Pointer{unsafe.Pointer(badge), unsafe.Pointer(up), unsafe.Pointer(down)} {
		C.free(p) // free(NULL) is a no-op
	}
}
