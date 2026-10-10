package app

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

// setMenuDecor gives a menu item a capsule badge (macOS 14+), plain grey text right-aligned after
// its label, and the Finder icon of path; NULL or "" clears each. Runs on the main thread.
static void setMenuDecor(void *p, const char *label, const char *badge, const char *detail, const char *path) {
	static NSMutableDictionary *icons;
	NSMenuItem *item = p;
	NSString *d = detail && *detail ? @(detail) : nil;
	if (!d) {
		if (item.attributedTitle) item.attributedTitle = nil;
	} else {
		NSString *l = @(label), *want = [NSString stringWithFormat:@"%@\t%@", l, d];
		if (![item.attributedTitle.string isEqualToString:want]) {
			NSMutableParagraphStyle *para = [[NSMutableParagraphStyle new] autorelease];
			para.tabStops = @[[[[NSTextTab alloc] initWithTextAlignment:NSTextAlignmentRight location:240 options:@{}] autorelease]];
			NSFont *f = [NSFont menuFontOfSize:0];
			NSMutableAttributedString *s = [[[NSMutableAttributedString alloc] initWithString:[l stringByAppendingString:@"\t"]
				attributes:@{NSFontAttributeName: f, NSParagraphStyleAttributeName: para}] autorelease];
			[s appendAttributedString:[[[NSAttributedString alloc] initWithString:d attributes:@{
				NSFontAttributeName: [NSFont monospacedDigitSystemFontOfSize:f.pointSize weight:NSFontWeightRegular],
				NSForegroundColorAttributeName: NSColor.secondaryLabelColor, NSParagraphStyleAttributeName: para}] autorelease]];
			item.attributedTitle = s;
		}
	}
	if (@available(macOS 14, *)) {
		NSString *b = badge && *badge ? @(badge) : nil;
		if (!b) {
			item.badge = nil;
		} else if (![item.badge.stringValue isEqualToString:b]) {
			item.badge = [[[NSMenuItemBadge alloc] initWithString:b] autorelease];
		}
	}
	NSImage *img = nil;
	if (path && *path) {
		if (!icons) icons = [NSMutableDictionary new];
		NSString *key = @(path);
		img = icons[key];
		if (!img) {
			img = [[[NSWorkspace sharedWorkspace] iconForFile:key] copy];
			img.size = NSMakeSize(16, 16);
			icons[key] = img;
			[img release];
		}
	}
	if (item.image != img) item.image = img;
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
func statusItem(tray *application.SystemTray) unsafe.Pointer { return native(tray, "nsStatusItem") }

// native reads field of the platform impl behind a Wails tray or menu item.
func native(v any, field string) unsafe.Pointer {
	impl := reflect.ValueOf(v).Elem().FieldByName("impl")
	if !impl.IsValid() || impl.IsNil() || impl.Elem().Kind() != reflect.Pointer {
		return nil
	}
	f := impl.Elem().Elem().FieldByName(field)
	if !f.IsValid() || f.Kind() != reflect.UnsafePointer {
		return nil
	}
	return unsafe.Pointer(f.Pointer())
}

// decorate draws the badges and icons Wails menus can't. On the main thread, which is also where
// Menu.Update swaps the NSMenuItems, so none is freed mid-call.
func decorate(badges, details, icons map[*application.MenuItem]string) {
	application.InvokeSync(func() {
		for _, items := range []map[*application.MenuItem]string{badges, details, icons} {
			for item := range items {
				p := native(item, "nsMenuItem")
				if p == nil {
					continue
				}
				strs := []*C.char{C.CString(item.Label()), C.CString(badges[item]), C.CString(details[item]), C.CString(icons[item])}
				C.setMenuDecor(p, strs[0], strs[1], strs[2], strs[3])
				for _, s := range strs {
					C.free(unsafe.Pointer(s))
				}
			}
		}
	})
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
func showTray(tray *application.SystemTray, icon []byte, t Traffic, mode string, capturing, showSpeed bool) {
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
	C.setTray(item, unsafe.Pointer(&icon[0]), C.int(len(icon)), badge, C.bool(!capturing), up, down)
	for _, p := range []unsafe.Pointer{unsafe.Pointer(badge), unsafe.Pointer(up), unsafe.Pointer(down)} {
		C.free(p) // free(NULL) is a no-op
	}
}
