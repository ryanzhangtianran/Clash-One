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

// DetailItemView draws a menu item that carries something on its right, as Surge's menu does: an
// optional app icon, the title, then grey text (a group's node, a process's speed) or a coloured
// pill (outbound mode, network quality) right-aligned up to a small gap before the submenu arrow.
// NSMenu can't lay that out itself (a title's text stops short of the arrow by the key-equivalent
// column), so these items are drawn whole; the rest of the menu stays native.
@interface DetailItemView : NSView
@property (copy) NSString *label, *detail; // detail: "text", or "\x01color|text" for a pill
@property (retain) NSImage *icon;
@end

static const CGFloat kInset = 5;      // the highlight's inset from the menu edge
static const CGFloat kTitleX = 16;    // where native titles start in this menu (no state column)
static const CGFloat kArrowW = 24;    // room for the submenu arrow at the right
static const CGFloat kItemH = 22;

static NSFont *menuFont(void) { return [NSFont menuFontOfSize:0]; }
static NSFont *monoFont(void) { return [NSFont monospacedDigitSystemFontOfSize:menuFont().pointSize - 1 weight:NSFontWeightRegular]; } // a node, a speed: a step below titles
static NSFont *pillFont(void) { return [NSFont monospacedDigitSystemFontOfSize:10 weight:NSFontWeightMedium]; }

@implementation DetailItemView
- (BOOL)isFlipped { return YES; }
- (BOOL)acceptsFirstMouse:(NSEvent *)e { return YES; }

- (BOOL)pill { return [self.detail hasPrefix:@"\x01"]; }
- (NSString *)pillText { return [[self.detail componentsSeparatedByString:@"|"] lastObject]; }
- (NSColor *)pillColor {
	NSString *c = [[self.detail substringFromIndex:1] componentsSeparatedByString:@"|"].firstObject;
	NSDictionary *colors = @{@"accent": NSColor.systemBlueColor, @"good": NSColor.systemGreenColor,
		@"warn": NSColor.systemOrangeColor, @"bad": NSColor.systemRedColor};
	return colors[c] ?: NSColor.systemGrayColor;
}
- (CGFloat)detailWidth {
	if (!self.detail.length) return 0;
	if (self.pill) return MAX(ceil([self.pillText sizeWithAttributes:@{NSFontAttributeName: pillFont()}].width) + 10, 18);
	return ceil([self.detail sizeWithAttributes:@{NSFontAttributeName: monoFont()}].width);
}
- (CGFloat)fittingWidth {
	CGFloat x = kTitleX + (self.icon ? 22 : 0);
	return x + ceil([self.label sizeWithAttributes:@{NSFontAttributeName: menuFont()}].width) + 24 + self.detailWidth + kArrowW + kInset;
}

- (void)drawRect:(NSRect)dirty {
	NSMenuItem *item = self.enclosingMenuItem;
	BOOL hi = item.isHighlighted && item.isEnabled;
	NSRect b = self.bounds;
	if (hi) {
		[NSColor.selectedContentBackgroundColor setFill];
		[[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(b, kInset, 0) xRadius:5 yRadius:5] fill];
	}
	NSColor *fg = !item.isEnabled ? NSColor.disabledControlTextColor : hi ? NSColor.selectedMenuItemTextColor : NSColor.labelColor;
	NSColor *dim = hi ? NSColor.selectedMenuItemTextColor : NSColor.secondaryLabelColor;
	CGFloat x = kTitleX;
	if (self.icon) {
		[self.icon drawInRect:NSMakeRect(x, (kItemH - 16) / 2, 16, 16) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
		x += 22;
	}
	NSDictionary *ta = @{NSFontAttributeName: menuFont(), NSForegroundColorAttributeName: fg};
	NSSize ts = [self.label sizeWithAttributes:ta];
	[self.label drawAtPoint:NSMakePoint(x, (kItemH - ts.height) / 2) withAttributes:ta];

	CGFloat right = NSMaxX(b) - kInset - (item.hasSubmenu ? kArrowW : 10); // without an arrow, near the edge like the arrows
	if (self.detail.length && self.pill) {
		CGFloat w = self.detailWidth, h = 16;
		NSRect r = NSMakeRect(right - w, (kItemH - h) / 2, w, h);
		[self.pillColor setFill];
		[[NSBezierPath bezierPathWithRoundedRect:r xRadius:4 yRadius:4] fill];
		NSDictionary *pa = @{NSFontAttributeName: pillFont(), NSForegroundColorAttributeName: NSColor.whiteColor};
		NSSize ps = [self.pillText sizeWithAttributes:pa];
		[self.pillText drawAtPoint:NSMakePoint(NSMidX(r) - ps.width / 2, NSMidY(r) - ps.height / 2) withAttributes:pa];
	} else if (self.detail.length) {
		NSDictionary *da = @{NSFontAttributeName: monoFont(), NSForegroundColorAttributeName: dim};
		NSSize ds = [self.detail sizeWithAttributes:da];
		[self.detail drawAtPoint:NSMakePoint(right - ds.width, (kItemH - ds.height) / 2) withAttributes:da];
	}
	if (item.hasSubmenu) {
		NSImage *chev = [NSImage imageWithSystemSymbolName:@"chevron.right" accessibilityDescription:nil];
		NSImageSymbolConfiguration *cfg = [NSImageSymbolConfiguration configurationWithPointSize:11 weight:NSFontWeightSemibold];
		chev = [chev imageWithSymbolConfiguration:[cfg configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithHierarchicalColor:fg]]];
		NSSize cs = chev.size;
		[chev drawInRect:NSMakeRect(NSMaxX(b) - kInset - 10 - cs.width, (kItemH - cs.height) / 2, cs.width, cs.height)
			fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
	}
}

// a click on a drawn item does what a native one would: close the menu and send its action
- (void)mouseUp:(NSEvent *)e {
	NSMenuItem *item = self.enclosingMenuItem;
	if (item.hasSubmenu || !item.isEnabled) return;
	NSMenu *m = item.menu;
	[m cancelTracking];
	[m performActionForItemAtIndex:[m indexOfItem:item]];
}
@end

// setMenuDecor draws an item with a right-side detail (grey text, or a "\x01color|text" pill)
// and the Finder icon of path, through DetailItemView; with neither it stays a native item.
// Runs on the main thread.
static void setMenuDecor(void *p, const char *label, const char *detail, const char *path) {
	static NSMutableDictionary *icons;
	NSMenuItem *item = p;
	NSString *d = detail && *detail ? @(detail) : nil;
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
	if (!d && !img) {
		if (item.view) item.view = nil;
		return;
	}
	DetailItemView *v = [item.view isKindOfClass:DetailItemView.class] ? (DetailItemView *)item.view : nil;
	if (!v) {
		v = [[[DetailItemView alloc] initWithFrame:NSMakeRect(0, 0, 100, kItemH)] autorelease];
		v.autoresizingMask = NSViewWidthSizable;
		item.view = v;
	}
	NSString *l = @(label);
	if (![v.label isEqualToString:l] || ![v.detail ?: @"" isEqualToString:d ?: @""] || v.icon != img) {
		v.label = l; v.detail = d; v.icon = img;
		[v setFrameSize:NSMakeSize(MAX(v.frame.size.width, v.fittingWidth), kItemH)];
		v.needsDisplay = YES;
	}
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
func decorate(details, icons map[*application.MenuItem]string) {
	application.InvokeSync(func() {
		for _, items := range []map[*application.MenuItem]string{details, icons} {
			for item := range items {
				p := native(item, "nsMenuItem")
				if p == nil {
					continue
				}
				strs := []*C.char{C.CString(item.Label()), C.CString(details[item]), C.CString(icons[item])}
				C.setMenuDecor(p, strs[0], strs[1], strs[2])
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
