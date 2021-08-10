#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>

typedef struct {
    int width;
    int height;
    int hotx;
    int hoty;
} cursor_metadata;

static int readCursor(char *imageOutput, cursor_metadata *metadataOutput) {
	NSAutoreleasePool *pool = [[NSAutoreleasePool alloc] init];
	NSCursor *currentSystemCursor = [NSCursor currentSystemCursor];
	NSPoint hotSpot = [currentSystemCursor hotSpot];
	NSImage *manyImage = [currentSystemCursor image];
	NSImageRep *imageRep = [[manyImage representations] objectAtIndex:0];
	// NSImageRep *imageRep = [manyImage bestRepresentationForRect:NSMakeRect(0, 0, 1024.0, 1024.0) context:nil hints:nil];
	NSImage * image = [[NSImage alloc] initWithSize:[imageRep size]];
	[image addRepresentation: imageRep];

	// NSLog(@"%lu", [[image representations] count]);

	metadataOutput->width = imageRep.pixelsWide;
	metadataOutput->height = imageRep.pixelsHigh;
	metadataOutput->hotx = hotSpot.x;
	metadataOutput->hoty = hotSpot.y;

	// CGImageRef cgRef = [image CGImageForProposedRect:NULL context:nil hints:nil];
	// NSBitmapImageRep *newRep = [[NSBitmapImageRep alloc] initWithCGImage:cgRef];
	// [newRep setSize:[image size]];   // if you want the same resolution
	// NSData *data = [newRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
	NSData *data = [image TIFFRepresentation];
	int len = [data length];
	[data getBytes:imageOutput length:len];

	[pool drain];

	return len;
}

