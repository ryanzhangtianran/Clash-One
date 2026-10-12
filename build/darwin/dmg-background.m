// dmg-background.m draws dmg-background.tiff, the install window behind make-dmg.sh:
//   clang -framework Cocoa dmg-background.m -o /tmp/bg && /tmp/bg /tmp/bg.png /tmp/bg@2x.png \
//   && tiffutil -cathidpicheck /tmp/bg.png /tmp/bg@2x.png -out dmg-background.tiff
#import <Cocoa/Cocoa.h>
static void draw(int scale, NSString *path) {
	int W = 640, H = 400;
	NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:W*scale pixelsHigh:H*scale bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
	rep.size = NSMakeSize(W, H);
	[NSGraphicsContext saveGraphicsState];
	NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
	NSGradient *bg = [[NSGradient alloc] initWithStartingColor:[NSColor colorWithSRGBRed:0.985 green:0.985 blue:0.992 alpha:1] endingColor:[NSColor colorWithSRGBRed:0.925 green:0.933 blue:0.953 alpha:1]];
	[bg drawInRect:NSMakeRect(0, 0, W, H) angle:-90];
	// y is from the bottom here; Finder places icons from the top: icons centred at y 200 from the top
	NSMutableParagraphStyle *center = [NSMutableParagraphStyle new]; center.alignment = NSTextAlignmentCenter;
	[@"Connaio" drawInRect:NSMakeRect(0, H - 84, W, 36) withAttributes:@{NSFontAttributeName: [NSFont systemFontOfSize:26 weight:NSFontWeightSemibold], NSForegroundColorAttributeName: [NSColor colorWithSRGBRed:0.114 green:0.114 blue:0.122 alpha:1], NSParagraphStyleAttributeName: center}];
	// three chevrons from the app to Applications, fading in towards it
	CGFloat y = H - 200;
	for (int i = 0; i < 3; i++) {
		CGFloat x = 296 + i * 22;
		NSBezierPath *c = [NSBezierPath bezierPath];
		[c moveToPoint:NSMakePoint(x - 6, y + 11)]; [c lineToPoint:NSMakePoint(x + 6, y)]; [c lineToPoint:NSMakePoint(x - 6, y - 11)];
		c.lineWidth = 2.5; c.lineCapStyle = NSLineCapStyleRound; c.lineJoinStyle = NSLineJoinStyleRound;
		[[NSColor colorWithSRGBRed:0.525 green:0.525 blue:0.545 alpha:0.3 + 0.25 * i] setStroke]; [c stroke];
	}
	[@"将 Connaio 拖到 Applications 完成安装" drawInRect:NSMakeRect(0, 44, W, 20) withAttributes:@{NSFontAttributeName: [NSFont systemFontOfSize:13 weight:NSFontWeightMedium], NSForegroundColorAttributeName: [NSColor colorWithSRGBRed:0.525 green:0.525 blue:0.545 alpha:1], NSParagraphStyleAttributeName: center}];
	[NSGraphicsContext restoreGraphicsState];
	[[rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}] writeToFile:path atomically:YES];
}
int main(int argc, char **argv) {
	@autoreleasepool { draw(1, @(argv[1])); draw(2, @(argv[2])); }
}
