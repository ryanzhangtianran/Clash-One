package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdlib.h>

// makeVibrant lets the window's blur through: Wails adds the effect view for a translucent window
// but leaves its default, all but opaque material, and its "transparent webview" call doesn't take
// on current macOS, so the webview still paints an opaque background over it.
// placeLights centres the close button on the floating sidebar's corner arc (the sidebar sits gap
// from the window edges with corner radius r), so the traffic lights are concentric with it;
// minimise and zoom follow at their usual spacing.
static void placeLights(NSWindow *w, CGFloat gap, CGFloat r) {
	NSButton *close = [w standardWindowButton:NSWindowCloseButton];
	NSView *bar = close.superview;
	if (!bar) return;
	CGFloat step = [w standardWindowButton:NSWindowMiniaturizeButton].frame.origin.x - close.frame.origin.x;
	NSWindowButton kinds[] = {NSWindowCloseButton, NSWindowMiniaturizeButton, NSWindowZoomButton};
	for (int i = 0; i < 3; i++) {
		NSButton *b = [w standardWindowButton:kinds[i]];
		NSSize sz = b.frame.size;
		CGFloat cx = gap + r + i * step, cy = gap + r; // centre, from the window's top-left
		[b setFrameOrigin:NSMakePoint(cx - sz.width / 2, NSHeight(bar.frame) - cy - sz.height / 2)];
	}
}

// keepLights re-places them whenever AppKit lays the title bar out again.
static void keepLights(NSWindow *w, CGFloat gap, CGFloat r) {
	placeLights(w, gap, r);
	for (NSNotificationName n in @[NSWindowDidResizeNotification, NSWindowDidEndLiveResizeNotification, NSWindowDidBecomeKeyNotification,
		NSWindowDidResignKeyNotification, NSWindowDidExitFullScreenNotification, NSWindowDidBecomeMainNotification]) {
		[NSNotificationCenter.defaultCenter addObserverForName:n object:w queue:NSOperationQueue.mainQueue usingBlock:^(NSNotification *note) {
			placeLights(w, gap, r);
		}];
	}
}

static void makeVibrant(void *w) {
	dispatch_async(dispatch_get_main_queue(), ^{
		keepLights((NSWindow *)w, 12, 14); // style.css: --gap and the sidebar's border-radius
		for (NSView *v in ((NSWindow *)w).contentView.subviews) {
			if ([v isKindOfClass:NSVisualEffectView.class]) {
				((NSVisualEffectView *)v).material = NSVisualEffectMaterialSidebar;
			} else if ([v isKindOfClass:WKWebView.class]) {
				[v setValue:@NO forKey:@"drawsBackground"];
				if (@available(macOS 12, *)) ((WKWebView *)v).underPageBackgroundColor = NSColor.clearColor;
			}
		}
	});
}

// pngOf draws img fitted into a px×px PNG (centred at its own size when fit is 0), in a malloc'd
// buffer of *n bytes, or NULL.
static void *pngOf(NSImage *img, int px, BOOL fit, int *n) {
	if (!img) return NULL;
	NSBitmapImageRep *rep = [[[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:px pixelsHigh:px
		bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0] autorelease];
	[NSGraphicsContext saveGraphicsState];
	NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
	NSSize s = fit ? NSMakeSize(px, px) : img.size;
	[img drawInRect:NSMakeRect((px - s.width) / 2, (px - s.height) / 2, s.width, s.height)];
	[NSGraphicsContext restoreGraphicsState];
	NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
	if (!png) return NULL;
	*n = (int)png.length;
	void *out = malloc(png.length);
	memcpy(out, png.bytes, png.length);
	return out;
}

// symbolPNG renders SF Symbol name in white into a px×px PNG.
static void *symbolPNG(const char *name, int px, int *n) {
	@autoreleasepool {
		NSImage *sym = [NSImage imageWithSystemSymbolName:@(name) accessibilityDescription:nil];
		NSImageSymbolConfiguration *cfg = [NSImageSymbolConfiguration configurationWithPointSize:px * 0.6 weight:NSFontWeightSemibold];
		cfg = [cfg configurationByApplyingConfiguration:[NSImageSymbolConfiguration configurationWithPaletteColors:@[NSColor.whiteColor]]];
		return pngOf([sym imageWithSymbolConfiguration:cfg], px, NO, n);
	}
}

// iconPNG is the Finder icon of path as a 64 px PNG.
static void *iconPNG(const char *path, int *n) {
	@autoreleasepool {
		return pngOf([[NSWorkspace sharedWorkspace] iconForFile:@(path)], 64, YES, n);
	}
}
*/
import "C"

import (
	"encoding/base64"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func makeVibrant(w *application.WebviewWindow) {
	if p := w.NativeWindow(); p != nil {
		C.makeVibrant(p)
	}
}

var icons sync.Map // key -> data URL

// AppIcon is the Finder icon of path (an app bundle, or an executable's generic icon) as a PNG data
// URL, for the activity page's client list.
func (s *SettingsService) AppIcon(path string) string {
	return cachedPNG(path, func(n *C.int) unsafe.Pointer {
		cs := C.CString(path)
		defer C.free(unsafe.Pointer(cs))
		return C.iconPNG(cs, n)
	})
}

// Symbol is SF Symbol name in white as a PNG data URL (rendered at 3× of the 14 pt the sidebar shows).
func (s *SettingsService) Symbol(name string) string {
	return cachedPNG("symbol:"+name, func(n *C.int) unsafe.Pointer {
		cs := C.CString(name)
		defer C.free(unsafe.Pointer(cs))
		return C.symbolPNG(cs, 48, n)
	})
}

// cachedPNG is render's malloc'd PNG as a data URL, cached under key; "" when render gives NULL.
func cachedPNG(key string, render func(n *C.int) unsafe.Pointer) string {
	if v, ok := icons.Load(key); ok {
		return v.(string)
	}
	var n C.int
	p := render(&n)
	if p == nil {
		return ""
	}
	defer C.free(p)
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(C.GoBytes(p, n))
	icons.Store(key, url)
	return url
}
