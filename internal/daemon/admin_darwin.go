package daemon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>

// runAppleScript runs src inside this app (not via osascript), so the administrator dialog it raises
// names Clash One and shows its icon. Returns NULL on success, else a malloc'd "number message".
static char *runAppleScript(const char *src) {
	__block char *res = NULL;
	void (^run)(void) = ^{
		@autoreleasepool {
			NSDictionary *err = nil;
			NSAppleScript *s = [[[NSAppleScript alloc] initWithSource:@(src)] autorelease];
			if (![s executeAndReturnError:&err]) {
				res = strdup([NSString stringWithFormat:@"%@ %@", err[NSAppleScriptErrorNumber], err[NSAppleScriptErrorMessage]].UTF8String);
			}
		}
	};
	// NSAppleScript belongs on the main thread; the window waits while the password dialog is up
	if (NSThread.isMainThread) run(); else dispatch_sync(dispatch_get_main_queue(), run);
	return res;
}
*/
import "C"

import "unsafe"

func runAppleScript(src string) string {
	cs := C.CString(src)
	defer C.free(unsafe.Pointer(cs))
	res := C.runAppleScript(cs)
	if res == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(res))
	return C.GoString(res)
}
