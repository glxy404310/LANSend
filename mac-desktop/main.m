#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import <math.h>

static NSColor *LSCanvasColor(void) { return NSColor.windowBackgroundColor; }
static NSColor *LSCardColor(void) { return NSColor.controlBackgroundColor; }
static NSColor *LSCardBorderColor(void) { return NSColor.separatorColor; }
static NSColor *LSDividerColor(void) { return NSColor.separatorColor; }
static NSColor *LSSegTrackColor(void) { return NSColor.unemphasizedSelectedContentBackgroundColor; }
static NSColor *LSInkPrimary(void) { return NSColor.labelColor; }
static NSColor *LSInkSecondary(void) { return NSColor.secondaryLabelColor; }
static NSColor *LSInkMuted(void) { return NSColor.disabledControlTextColor; }
static NSColor *LSAccentColor(void) { return NSColor.controlAccentColor; }
static NSColor *LSSecondaryBorder(void) { return NSColor.separatorColor; }
static NSColor *LSSecondaryHover(void) { return NSColor.unemphasizedSelectedContentBackgroundColor; }
static NSColor *LSSecondaryPressed(void) { return NSColor.selectedControlColor; }
static NSColor *LSDisabledBg(void) { return NSColor.controlBackgroundColor; }
static NSColor *LSDisabledBorder(void) { return NSColor.separatorColor; }
static CGFloat linearChannel(CGFloat x) { return x <= 0.04045 ? x/12.92 : pow((x+0.055)/1.055,2.4); }
static NSColor *LSAccentText(NSColor *fill) {
    NSColor *rgb=[fill colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
    CGFloat light=0.2126*linearChannel(rgb.redComponent)+0.7152*linearChannel(rgb.greenComponent)+0.0722*linearChannel(rgb.blueComponent);
    return light>0.179 ? NSColor.blackColor : NSColor.whiteColor;
}

@interface LSSurface : NSView
@property(strong) NSColor *fill;
@end
@implementation LSSurface
- (void)drawRect:(NSRect)dirty { [self.fill setFill];NSRectFill(dirty); }
@end

@interface LSCardView : NSView
@property(nonatomic) CGFloat radius;
@end
@implementation LSCardView
- (instancetype)initWithFrame:(NSRect)frame {
    if ((self = [super initWithFrame:frame])) {
        self.wantsLayer = YES;
        self.radius = 12.0;
        self.layer.cornerRadius = self.radius;
        self.layer.masksToBounds = YES;
    }
    return self;
}
- (BOOL)wantsUpdateLayer { return YES; }
- (void)updateLayer {
    self.layer.backgroundColor=LSCardColor().CGColor;
    self.layer.borderColor=(self.layer.borderWidth>1 ? LSAccentColor() : LSCardBorderColor()).CGColor;
    if(self.layer.borderWidth==0)self.layer.borderWidth=1;
}
- (void)viewDidChangeEffectiveAppearance { [super viewDidChangeEffectiveAppearance];self.needsDisplay=YES; }
@end

@interface LSButton : NSButton
@property(nonatomic) BOOL primary;
@property(nonatomic) BOOL hovered;
@property(nonatomic) BOOL pressed;
@property(nonatomic) CGFloat cornerRadius;
@end
@implementation LSButton
- (instancetype)initWithFrame:(NSRect)frame {
    if ((self = [super initWithFrame:frame])) {
        self.bordered = NO;
        self.bezelStyle = NSBezelStyleRegularSquare;
        self.cornerRadius = 8.0;
        self.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
        self.wantsLayer = YES;
        [self updateTracking];
    }
    return self;
}
- (void)updateTracking {
    for (NSTrackingArea *a in self.trackingAreas) [self removeTrackingArea:a];
    NSTrackingArea *area = [[NSTrackingArea alloc] initWithRect:self.bounds
        options:NSTrackingMouseEnteredAndExited|NSTrackingActiveAlways|NSTrackingInVisibleRect
        owner:self userInfo:nil];
    [self addTrackingArea:area];
}
- (void)mouseEntered:(NSEvent *)event { self.hovered = YES; [self setNeedsDisplay:YES]; }
- (void)mouseExited:(NSEvent *)event  { self.hovered = NO;  self.pressed = NO; [self setNeedsDisplay:YES]; }
- (void)mouseDown:(NSEvent *)event {
    if (!self.enabled) return;
    self.pressed = YES;
    [self setNeedsDisplay:YES];
    [super mouseDown:event];
    self.pressed = NO;
    [self setNeedsDisplay:YES];
}
- (void)setEnabled:(BOOL)enabled {
    [super setEnabled:enabled];
    [self setNeedsDisplay:YES];
}
- (void)drawRect:(NSRect)dirtyRect {
    NSRect r = NSInsetRect(self.bounds, 0.5, 0.5);
    NSBezierPath *path = [NSBezierPath bezierPathWithRoundedRect:r xRadius:self.cornerRadius yRadius:self.cornerRadius];
    NSColor *bg, *border, *fg;
    BOOL drawBorder = YES;
    if (self.primary) {
        drawBorder = NO;
        if (!self.enabled) {
            bg = LSDisabledBg(); border = LSDisabledBorder(); fg = LSInkMuted(); drawBorder = YES;
        } else if (self.pressed) {
            bg = [NSColor.systemBlueColor shadowWithLevel:0.15]; border = bg; fg = LSAccentText(bg);
        } else if (self.hovered) {
            bg = [NSColor.systemBlueColor highlightWithLevel:0.10]; border = bg; fg = LSAccentText(bg);
        } else {
            bg = NSColor.systemBlueColor; border = bg; fg = LSAccentText(bg);
        }
    } else {
        if (!self.enabled) {
            bg = LSDisabledBg(); border = LSDisabledBorder(); fg = LSInkMuted();
        } else if (self.pressed) {
            bg = LSSecondaryPressed(); border = LSSecondaryBorder(); fg = LSInkPrimary();
        } else if (self.hovered) {
            bg = LSSecondaryHover(); border = LSSecondaryBorder(); fg = LSInkPrimary();
        } else {
            bg = LSCardColor(); border = LSSecondaryBorder(); fg = LSInkPrimary();
        }
    }
    [bg setFill];
    [path fill];
    if (drawBorder) {
        [border setStroke];
        path.lineWidth = 1.0;
        [path stroke];
    }
    NSDictionary *attrs = @{
        NSFontAttributeName: self.font ?: [NSFont systemFontOfSize:13 weight:NSFontWeightMedium],
        NSForegroundColorAttributeName: fg,
    };
    NSSize sz = [self.title sizeWithAttributes:attrs];
    NSRect tr = NSMakeRect(round((self.bounds.size.width - sz.width) / 2.0),
                           round((self.bounds.size.height - sz.height) / 2.0) - 0.5,
                           sz.width, sz.height);
    [self.title drawInRect:tr withAttributes:attrs];
}
@end

@interface LSStatusBadge : NSView
@property(nonatomic) NSInteger connectedCount;
@end
@implementation LSStatusBadge
- (void)setConnectedCount:(NSInteger)count {
    _connectedCount=count;
    [self setNeedsDisplay:YES];
}
- (void)drawRect:(NSRect)dirtyRect {
    NSString *label=self.connectedCount<0 ? @"已连接设备：—" : [NSString stringWithFormat:@"已连接 %ld 台设备",(long)self.connectedCount];
    BOOL paired=self.connectedCount>0;
    NSColor *bg     = paired ? [LSAccentColor() colorWithAlphaComponent:.14] : LSSegTrackColor();
    NSColor *border = paired ? LSAccentColor() : LSCardBorderColor();
    NSColor *dot    = paired ? LSAccentColor()  : LSInkMuted();
    NSColor *fg     = paired ? LSInkPrimary()    : LSInkSecondary();
    NSDictionary *attrs = @{
        NSFontAttributeName: [NSFont systemFontOfSize:12 weight:NSFontWeightMedium],
        NSForegroundColorAttributeName: fg,
    };
    NSSize sz = [label sizeWithAttributes:attrs];
    CGFloat padX = 11.0, dotD = 7.0, gap = 6.0;
    CGFloat pillH = 26.0;
    CGFloat pillW = MIN(self.bounds.size.width, ceil(padX + dotD + gap + sz.width + padX));
    NSRect pr = NSMakeRect(self.bounds.size.width - pillW, round((self.bounds.size.height - pillH) / 2.0), pillW, pillH);
    NSBezierPath *pill = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(pr, 0.5, 0.5) xRadius:pillH/2.0 yRadius:pillH/2.0];
    [bg setFill]; [pill fill];
    [border setStroke]; pill.lineWidth = 1.0; [pill stroke];
    NSRect dr = NSMakeRect(pr.origin.x + padX, pr.origin.y + round((pillH - dotD) / 2.0), dotD, dotD);
    [dot setFill];
    [[NSBezierPath bezierPathWithOvalInRect:dr] fill];
    CGFloat maxTextW = MAX(0, pillW - padX * 2 - dotD - gap);
    NSRect tr = NSMakeRect(NSMaxX(dr) + gap, pr.origin.y + round((pillH - sz.height) / 2.0) - 0.5, maxTextW, sz.height);
    [label drawWithRect:tr options:NSStringDrawingUsesLineFragmentOrigin|NSStringDrawingTruncatesLastVisibleLine attributes:attrs];
}
@end

@interface LSSegmentedBar : NSControl
@property(nonatomic, strong) NSArray<NSString *> *labels;
@property(nonatomic) NSInteger selectedSegment;
@property(nonatomic) NSInteger hoveredSegment;
@end
@implementation LSSegmentedBar
- (instancetype)initWithFrame:(NSRect)frame labels:(NSArray<NSString *> *)labels target:(id)target action:(SEL)action {
    if ((self = [super initWithFrame:frame])) {
        self.labels = labels;
        self.target = target;
        self.action = action;
        self.selectedSegment = 0;
        self.hoveredSegment = -1;
        NSTrackingArea *area = [[NSTrackingArea alloc] initWithRect:self.bounds
            options:NSTrackingMouseEnteredAndExited|NSTrackingMouseMoved|NSTrackingActiveAlways|NSTrackingInVisibleRect
            owner:self userInfo:nil];
        [self addTrackingArea:area];
    }
    return self;
}
- (void)setSelectedSegment:(NSInteger)selectedSegment {
    _selectedSegment = selectedSegment;
    [self setNeedsDisplay:YES];
}
- (NSInteger)segmentAtPoint:(NSPoint)p {
    if (!self.labels.count || !NSPointInRect(p, self.bounds)) return -1;
    CGFloat pad = 4.0;
    CGFloat w = (self.bounds.size.width - pad * 2) / self.labels.count;
    NSInteger idx = (NSInteger)floor((p.x - pad) / w);
    if (idx < 0) idx = 0;
    if (idx >= (NSInteger)self.labels.count) idx = self.labels.count - 1;
    return idx;
}
- (void)mouseMoved:(NSEvent *)event {
    NSInteger idx = [self segmentAtPoint:[self convertPoint:event.locationInWindow fromView:nil]];
    if (idx != self.hoveredSegment) { self.hoveredSegment = idx; [self setNeedsDisplay:YES]; }
}
- (void)mouseExited:(NSEvent *)event {
    if (self.hoveredSegment != -1) { self.hoveredSegment = -1; [self setNeedsDisplay:YES]; }
}
- (void)mouseDown:(NSEvent *)event {
    NSInteger idx = [self segmentAtPoint:[self convertPoint:event.locationInWindow fromView:nil]];
    if (idx >= 0 && idx != self.selectedSegment) {
        self.selectedSegment = idx;
        if (self.target && self.action) [self sendAction:self.action to:self.target];
    }
}
- (void)drawRect:(NSRect)dirtyRect {
    NSBezierPath *track = [NSBezierPath bezierPathWithRoundedRect:self.bounds xRadius:10.0 yRadius:10.0];
    [LSSegTrackColor() setFill];
    [track fill];
    if (!self.labels.count) return;
    CGFloat pad = 4.0, gap = 3.0;
    CGFloat segW = (self.bounds.size.width - pad * 2 - gap * (self.labels.count - 1)) / self.labels.count;
    CGFloat segH = self.bounds.size.height - pad * 2;
    for (NSInteger i = 0; i < (NSInteger)self.labels.count; i++) {
        NSRect sr = NSMakeRect(pad + i * (segW + gap), pad, segW, segH);
        BOOL active = (i == self.selectedSegment);
        BOOL hovered = (i == self.hoveredSegment);
        if (active) {
            NSBezierPath *pill = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(sr, 0.5, 0.5) xRadius:7.0 yRadius:7.0];
            [LSCardColor() setFill]; [pill fill];
            [LSCardBorderColor() setStroke]; pill.lineWidth = 1.0; [pill stroke];
        } else if (hovered) {
            NSBezierPath *pill = [NSBezierPath bezierPathWithRoundedRect:sr xRadius:7.0 yRadius:7.0];
            [LSSecondaryHover() setFill]; [pill fill];
        }
        NSDictionary *attrs = @{
            NSFontAttributeName: [NSFont systemFontOfSize:12.5 weight:active ? NSFontWeightMedium : NSFontWeightRegular],
            NSForegroundColorAttributeName: (active || hovered) ? LSInkPrimary() : LSInkSecondary(),
        };
        NSSize sz = [self.labels[i] sizeWithAttributes:attrs];
        NSRect tr = NSMakeRect(sr.origin.x + round((sr.size.width - sz.width) / 2.0),
                               sr.origin.y + round((sr.size.height - sz.height) / 2.0) - 0.5,
                               sz.width, sz.height);
        [self.labels[i] drawInRect:tr withAttributes:attrs];
    }
}
@end

@interface TransferDropView : NSView
@property(copy) void (^enqueue)(NSArray<NSURL *> *);
@property(nonatomic, weak) LSCardView *dropHighlightCard;
@end
@implementation TransferDropView
- (NSDragOperation)draggingEntered:(id<NSDraggingInfo>)sender {
    BOOL ok = [sender.draggingPasteboard canReadObjectForClasses:@[NSURL.class] options:@{NSPasteboardURLReadingFileURLsOnlyKey:@YES}];
    if (ok && self.dropHighlightCard) {
        self.dropHighlightCard.layer.borderColor = LSAccentColor().CGColor;
        self.dropHighlightCard.layer.borderWidth = 1.5;
    }
    return ok ? NSDragOperationCopy : NSDragOperationNone;
}
- (void)draggingExited:(id<NSDraggingInfo>)sender {
    if (self.dropHighlightCard) {
        self.dropHighlightCard.layer.borderColor = LSCardBorderColor().CGColor;
        self.dropHighlightCard.layer.borderWidth = 1.0;
    }
}
- (BOOL)performDragOperation:(id<NSDraggingInfo>)sender {
    [self draggingExited:sender];
    NSArray *urls=[sender.draggingPasteboard readObjectsForClasses:@[NSURL.class] options:@{NSPasteboardURLReadingFileURLsOnlyKey:@YES}];
    if (!urls.count) return NO;
    self.enqueue(urls); return YES;
}
@end

@interface LSDeviceList : NSObject <NSTableViewDataSource>
@property(strong) NSMutableArray<NSMutableDictionary *> *rows;
@property(copy) NSString *target;
@end
@implementation LSDeviceList
- (NSInteger)numberOfRowsInTableView:(NSTableView *)table { return self.rows.count; }
- (id)tableView:(NSTableView *)table objectValueForTableColumn:(NSTableColumn *)column row:(NSInteger)i {
    NSDictionary *row=self.rows[i];
    if ([column.identifier isEqual:@"target"]) return [row[@"PeerHash"] isEqual:self.target] ? @"当前目标" : @"";
    return row[column.identifier];
}
- (void)resolveTarget {
    NSString *first=nil;
    for (NSDictionary *row in self.rows) {
        if (![row[@"Enabled"] boolValue]) continue;
        if ([row[@"PeerHash"] isEqual:self.target]) return;
        if (!first) first=row[@"PeerHash"];
    }
    self.target=first;
}
- (void)tableView:(NSTableView *)table setObjectValue:(id)value forTableColumn:(NSTableColumn *)column row:(NSInteger)i {
    self.rows[i][column.identifier]=value;
    if ([column.identifier isEqual:@"Enabled"]) {
        [self resolveTarget];
        [table reloadDataForRowIndexes:[NSIndexSet indexSetWithIndexesInRange:NSMakeRange(0,self.rows.count)]
                         columnIndexes:[NSIndexSet indexSetWithIndex:[table columnWithIdentifier:@"target"]]];
    }
}
@end
@interface Desktop:NSObject <NSApplicationDelegate,NSWindowDelegate,NSTextFieldDelegate>
@property NSWindow *window;
@property LSStatusBadge *peerBadge;
@property NSTextField *status,*receiveDirField;
@property LSButton *sendButton,*pairButton,*resetDirButton,*retryButton;
@property NSProgressIndicator *progress;
@property NSView *transfer,*settings,*devices;
@property NSPopUpButton *picker;
@property NSTextField *deviceHint;
@property BOOL scanning;
@property NSArray *lastDevices;
@property NSTimer *scanTimer;
@property BOOL probing;
@property BOOL sending;
@property NSTextField *nameField,*nameLabel,*pairedCount;
@property LSButton *editNameButton,*cancelNameButton;
@property NSTextView *queueView;
@property NSMutableArray<NSMutableDictionary *> *queue;
@property NSMutableArray<NSURL *> *pendingOpen;
@property BOOL readyToSend;
@property BOOL busy;
@property NSString *engine;
@property NSTask *receiver;
@property NSPanel *receipt;
@property NSPopUpButton *files;
@property NSTextField *receiptTitle;
@property NSPipe *events;
@end
@implementation Desktop
- (NSTextField*)label:(NSString*)value frame:(NSRect)rect size:(CGFloat)size bold:(BOOL)bold color:(NSColor*)color view:(NSView*)parent {
    NSTextField *f=[NSTextField labelWithString:value]; f.frame=rect;
    f.font=[NSFont systemFontOfSize:size weight:bold?NSFontWeightSemibold:NSFontWeightRegular];
    f.textColor=color?:LSInkPrimary();
    f.lineBreakMode=NSLineBreakByTruncatingMiddle; [parent addSubview:f];return f;
}
- (LSButton*)button:(NSString*)title at:(NSRect)rect action:(SEL)action primary:(BOOL)primary view:(NSView*)parent {
    LSButton *b=[[LSButton alloc] initWithFrame:rect];
    b.title=title; b.target=self; b.action=action; b.primary=primary;
    [parent addSubview:b];
    return b;
}
- (NSTextField*)styledFieldWithFrame:(NSRect)rect editable:(BOOL)editable view:(NSView*)parent {
    NSTextField *f=[[NSTextField alloc] initWithFrame:rect];
    f.editable=editable; f.selectable=YES; f.bezeled=YES; f.bezelStyle=NSTextFieldRoundedBezel;
    f.font=[NSFont systemFontOfSize:13 weight:NSFontWeightRegular];
    f.textColor=LSInkPrimary();
    f.lineBreakMode=NSLineBreakByTruncatingMiddle;
    [parent addSubview:f];
    return f;
}
- (void)refreshPairedCount {
    NSDictionary *first=[self selectedRecord];
    NSUInteger count=[first[@"PeerHash"] length]?1+[first[@"Others"] count]:0;
    self.pairedCount.stringValue=[NSString stringWithFormat:@"%lu 台",(unsigned long)count];
}
- (void)refreshPeer {
    if (self.pairedCount && !self.settings.hidden) [self refreshPairedCount];
    if (!self.engine || self.probing || !self.window.visible) return;
    self.probing=YES;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0), ^{
        NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:self.engine];task.arguments=@[@"connected-peer-count"];
        NSPipe *pipe=[NSPipe pipe];task.standardOutput=pipe;
        NSString *text=nil;
        if ([task launchAndReturnError:nil]){
            NSData *data=[pipe.fileHandleForReading readDataToEndOfFile];[task waitUntilExit];
            if (!task.terminationStatus) text=[[[NSString alloc]initWithData:data encoding:NSUTF8StringEncoding] stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
        }
        dispatch_async(dispatch_get_main_queue(), ^{self.probing=NO;self.peerBadge.connectedCount=text.length?text.integerValue:-1;});
    });
}
- (void)run:(NSString*)program args:(NSArray*)args input:(NSString*)input done:(void(^)(BOOL,NSString*))done {
    self.busy=YES;self.sendButton.enabled=self.sending;self.pairButton.enabled=NO;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
        NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:program];task.arguments=args;
        NSPipe *output=[NSPipe pipe];task.standardOutput=output;task.standardError=output;
        NSPipe *feed=[NSPipe pipe];task.standardInput=feed;
        NSError *e=nil; BOOL ok=[task launchAndReturnError:&e];NSMutableData *tail=[NSMutableData data];
        if(ok){
            if(input)[feed.fileHandleForWriting writeData:[[input stringByAppendingString:@"\n"] dataUsingEncoding:NSUTF8StringEncoding]];
            [feed.fileHandleForWriting closeFile];
            while(YES){NSData *part=output.fileHandleForReading.availableData;if(!part.length)break;
                [tail appendData:part];if(tail.length>131072)[tail replaceBytesInRange:NSMakeRange(0,tail.length-65536) withBytes:NULL length:0];
                if([args.firstObject isEqual:@"send-to"]){NSString *s=[[NSString alloc]initWithData:part encoding:NSUTF8StringEncoding];
                    NSString *line=[[s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet].lastObject;
                    if(line.length)dispatch_async(dispatch_get_main_queue(),^{self.status.stringValue=line;});
                }
            }
            [task waitUntilExit];ok=task.terminationStatus==0;
        }
        NSString *message=e.localizedDescription?:([[NSString alloc]initWithData:tail encoding:NSUTF8StringEncoding]?:@"");
        dispatch_async(dispatch_get_main_queue(),^{self.busy=NO;self.sendButton.enabled=YES;self.pairButton.enabled=YES;[self.progress stopAnimation:nil];done(ok,[message stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]);[self drainPending];});
    });
}
- (void)error:(NSString*)msg {NSAlert *a=[NSAlert new];a.messageText=@"操作未完成";a.informativeText=msg;[a beginSheetModalForWindow:self.window completionHandler:nil];}
- (void)refreshQueue {
    NSMutableArray *rows=[NSMutableArray array];BOOL failed=NO;
    for (NSDictionary *item in self.queue) {
        failed |= [item[@"state"] isEqual:@"失败"];
        NSString *detail=item[@"detail"]?:@"";
        if (detail.length) [rows addObject:[NSString stringWithFormat:@"%@   %@   %@",item[@"state"],[item[@"path"] lastPathComponent],detail]];
        else [rows addObject:[NSString stringWithFormat:@"%@   %@",item[@"state"],[item[@"path"] lastPathComponent]]];
    }
    self.queueView.string=[rows componentsJoinedByString:@"\n"]; self.retryButton.enabled=failed&&!self.busy;
}
- (void)nextItem {
    NSMutableDictionary *next=nil;NSUInteger failed=0;
    for (NSMutableDictionary *item in self.queue){
        if ([item[@"state"] isEqual:@"失败"])failed++;
        if (!next && [item[@"state"] isEqual:@"等待"])next=item;
    }
    if (!next) {
        self.sending=NO;[self refreshQueue];
        self.status.stringValue=[NSString stringWithFormat:@"已完成 %lu 项，失败 %lu 项",(unsigned long)(self.queue.count-failed),(unsigned long)failed];
        return;
    }
    self.sending=YES;next[@"state"]=@"发送中";self.retryButton.enabled=NO;[self refreshQueue];
    self.status.stringValue=[@"正在发送 " stringByAppendingString:[next[@"path"] lastPathComponent]];
    [self.progress startAnimation:nil];
    [self run:self.engine args:@[@"send-to",next[@"hash"],next[@"path"]] input:nil done:^(BOOL ok,NSString *msg){
        next[@"state"]=ok?@"已完成":@"失败";
        next[@"detail"]=ok?@"":([msg componentsSeparatedByString:@"\n"].lastObject?:@"");
        [self nextItem];
    }];
}
- (void)enqueueURLs:(NSArray<NSURL *> *)urls {
    if (!self.readyToSend || (self.busy && !self.sending)){
        if (!self.pendingOpen) self.pendingOpen=[NSMutableArray array];
        [self.pendingOpen addObjectsFromArray:urls];return;
    }
    NSDictionary *record=[self selectedRecord];if(![record[@"Enabled"] boolValue]){[self error:@"请先在设备管理中勾选启用发送目标。"];return;}
    for (NSURL *url in urls) if(url.isFileURL)[self.queue addObject:[@{@"hash":record[@"PeerHash"],@"path":url.path,@"state":@"等待"} mutableCopy]];
    if (self.sending)[self refreshQueue];else if(urls.count)[self nextItem];
}
- (void)drainPending {
    if (self.readyToSend && !self.busy && self.pendingOpen.count){
        NSArray *pending=[self.pendingOpen copy];[self.pendingOpen removeAllObjects];[self enqueueURLs:pending];
    }
}
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
    [self enqueueURLs:urls];[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
}
- (void)retryFailed:(id)sender {
    if(self.busy)return;
    for (NSMutableDictionary *item in self.queue)if([item[@"state"] isEqual:@"失败"]){item[@"state"]=@"等待";item[@"detail"]=@"";}
    [self nextItem];
}
- (void)send:(id)sender {
    NSOpenPanel *p=NSOpenPanel.openPanel;p.allowsMultipleSelection=YES;p.canChooseDirectories=YES;p.canChooseFiles=YES;p.prompt=@"加入发送队列";
    [p beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse result){if(result==NSModalResponseOK)[self enqueueURLs:p.URLs];}];
}
- (NSDictionary *)selectedRecord {
    NSString *path=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/peer.json"];
    NSData *data=[NSData dataWithContentsOfFile:path];
    return data?[NSJSONSerialization JSONObjectWithData:data options:0 error:nil]:nil;
}
- (void)applyDeviceCommands:(NSArray<NSArray *> *)commands index:(NSUInteger)i {
    if (i==commands.count) { [self refreshPeer]; return; }
    [self run:self.engine args:commands[i] input:nil done:^(BOOL ok,NSString *message){
        if (ok) [self applyDeviceCommands:commands index:i+1];
        else { [self refreshPeer]; [self error:message]; }
    }];
}
- (void)editNickname:(id)sender {
    if (self.busy) return;
    NSDictionary *first=[self selectedRecord];
    NSArray *peers=[first[@"PeerHash"] length] ? [@[first] arrayByAddingObjectsFromArray:first[@"Others"]?:@[]] : @[];
    LSDeviceList *list=[LSDeviceList new];
    list.rows=[NSMutableArray array];
    NSString *root=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend"];
    for (NSDictionary *peer in peers) {
        NSString *key=peer[@"PeerHash"];
        if (key.length!=64) { [self error:@"配对记录无效"]; return; }
        NSString *path=[root stringByAppendingPathComponent:[@"nickname-" stringByAppendingString:key]];
        NSString *note=[NSString stringWithContentsOfFile:path encoding:NSUTF8StringEncoding error:nil]?:@"";
        [list.rows addObject:[@{@"PeerHash":key, @"PeerName":[peer[@"PeerName"] length]?peer[@"PeerName"]:@"已配对设备",
            @"Enabled":@([peer[@"Enabled"] boolValue]), @"originalEnabled":@([peer[@"Enabled"] boolValue]),
            @"note":note, @"originalNote":note} mutableCopy]];
    }
    [list resolveTarget];
    NSString *originalTarget=list.target;
    NSAlert *alert=[NSAlert new];
    alert.messageText=@"设备管理";
    alert.informativeText=peers.count?@"勾选设备以允许收发。双击备注即可编辑。":@"暂无已配对设备";
    NSScrollView *scroll=[[NSScrollView alloc] initWithFrame:NSMakeRect(0,0,510,184)];
    scroll.hasVerticalScroller=YES;
    scroll.borderType=NSBezelBorder;
    NSTableView *table=[[NSTableView alloc] initWithFrame:scroll.bounds];
    table.rowHeight=28;
    for (NSArray *spec in @[@[@"Enabled",@"启用",@48],@[@"PeerName",@"设备名称",@140],@[@"note",@"备注",@200],@[@"target",@"发送目标",@92]]) {
        NSTableColumn *col=[[NSTableColumn alloc] initWithIdentifier:spec[0]];
        col.title=spec[1]; col.width=[spec[2] doubleValue];
        col.editable=[spec[0] isEqual:@"note"] || [spec[0] isEqual:@"Enabled"];
        if ([spec[0] isEqual:@"Enabled"]) {
            NSButtonCell *check=[NSButtonCell new];
            [check setButtonType:NSSwitchButton]; check.title=@""; col.dataCell=check;
        }
        [table addTableColumn:col];
    }
    table.dataSource=list;
    scroll.documentView=table;
    alert.accessoryView=scroll;
    [alert addButtonWithTitle:@"保存"];
    [alert addButtonWithTitle:@"设为发送目标"];
    [alert addButtonWithTitle:@"取消配对"];
    [alert addButtonWithTitle:@"关闭"].keyEquivalent=@"\x1b";
    NSMutableArray *removals=[NSMutableArray array];
    while (YES) {
        NSModalResponse response=[alert runModal];
        [alert.window makeFirstResponder:table];
        if (response==NSAlertFirstButtonReturn) {
            NSUInteger invalid=[list.rows indexOfObjectPassingTest:^BOOL(NSDictionary *row, NSUInteger index, BOOL *stop) {
                NSString *note=[row[@"note"] stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
                return [note lengthOfBytesUsingEncoding:NSUTF32LittleEndianStringEncoding]/4>48 ||
                       [note rangeOfCharacterFromSet:NSCharacterSet.controlCharacterSet].location!=NSNotFound;
            }];
            if (invalid==NSNotFound) break;
            alert.informativeText=@"设备备注最多可填写 48 个可显示字符。";
            [table selectRowIndexes:[NSIndexSet indexSetWithIndex:invalid] byExtendingSelection:NO];
            [table scrollRowToVisible:invalid];
            continue;
        }
        if (response!=NSAlertSecondButtonReturn && response!=NSAlertThirdButtonReturn) return;
        NSInteger i=table.selectedRow;
        if (i<0) { alert.informativeText=@"请先选中列表中的设备。"; continue; }
        NSDictionary *row=list.rows[i];
        if (response==NSAlertSecondButtonReturn) {
            if (![row[@"Enabled"] boolValue]) { alert.informativeText=@"请先勾选启用此设备。"; continue; }
            list.target=row[@"PeerHash"];
        } else {
            NSAlert *confirm=[NSAlert new];
            confirm.messageText=[NSString stringWithFormat:@"保存时取消与「%@」的配对？",row[@"PeerName"]];
            [confirm addButtonWithTitle:@"保留"]; [confirm addButtonWithTitle:@"取消配对"];
            if ([confirm runModal]!=NSAlertSecondButtonReturn) continue;
            [removals addObject:@[@"unpair",row[@"PeerHash"]]];
            [list.rows removeObjectAtIndex:i];
            [list resolveTarget];
        }
        [table reloadData];
    }
    NSMutableArray *commands=[NSMutableArray array];
    for (NSDictionary *row in list.rows) {
        NSString *key=row[@"PeerHash"];
        if (![row[@"note"] isEqual:row[@"originalNote"]])
            [commands addObject:@[@"set-peer-nickname",key,row[@"note"]]];
        if (![row[@"Enabled"] isEqual:row[@"originalEnabled"]])
            [commands addObject:@[[row[@"Enabled"] boolValue]?@"enable-peer":@"disable-peer",key]];
    }
    [commands addObjectsFromArray:removals];
    if (list.target && ![list.target isEqual:originalTarget]) [commands addObject:@[@"select-peer",list.target]];
    [self applyDeviceCommands:commands index:0];
}
- (void)setEditingName:(BOOL)editing {
    self.nameLabel.hidden=editing;
    self.nameField.hidden=!editing;
    self.cancelNameButton.hidden=!editing;
    self.editNameButton.title=editing?@"保存":@"编辑";
    if (editing) {
        self.nameField.stringValue=self.nameLabel.stringValue;
        [self.nameField selectText:nil];
    } else {
        [self.window makeFirstResponder:self.editNameButton];
    }
}
- (void)cancelNameEdit:(id)sender { [self setEditingName:NO]; }
- (BOOL)control:(NSControl *)control textView:(NSTextView *)view doCommandBySelector:(SEL)command {
    if (control==self.nameField && command==@selector(cancelOperation:)) {
        [self setEditingName:NO];
        return YES;
    }
    return NO;
}
- (void)saveDeviceName:(id)sender {
    if (self.busy) return;
    if (self.nameField.hidden) { [self setEditingName:YES]; return; }
    NSString *name=[self.nameField.stringValue stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
    [self run:self.engine args:@[@"set-device-name",name] input:nil done:^(BOOL ok,NSString *msg){
        if (!ok) { [self error:msg]; return; }
        self.nameLabel.stringValue=name;
        [self setEditingName:NO];
    }];
}
- (void)copyInvite:(id)sender { [self useInvite:YES]; }
- (void)pasteInvite:(id)sender { [self useInvite:NO]; }
- (void)useInvite:(BOOL)copy {
    if(self.busy)return;
    NSString *input=copy?nil:[NSPasteboard.generalPasteboard stringForType:NSPasteboardTypeString];
    if(!copy && !input.length){[self error:@"请先复制对方的 LANSend 码。"];return;}
    NSString *path=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/peer.json"];
    NSData *data=[NSData dataWithContentsOfFile:path];
    NSDictionary *peer=data?[NSJSONSerialization JSONObjectWithData:data options:0 error:nil]:nil;
    NSString *expected=peer[@"PeerHash"]?:@"";
    [self run:self.engine args:@[copy?@"invite-new":@"invite-join",expected] input:input done:^(BOOL ok,NSString *msg){
        if(!ok){[self error:msg];return;}
        if(copy){
            [NSPasteboard.generalPasteboard clearContents];
            if(![NSPasteboard.generalPasteboard setString:msg forType:NSPasteboardTypeString]){[self error:@"复制配对码失败"];return;}
        }
        [self refreshPeer];
        NSAlert *alert=[NSAlert new];alert.messageText=copy?@"已复制 LANSend 码":@"已双向配对";
        alert.informativeText=copy?@"请在另一台电脑点击「从剪贴板配对」。配对码 5 分钟内有效。":@"请在两端的设备管理中勾选启用对方。";
        [alert beginSheetModalForWindow:self.window completionHandler:nil];
    }];
}
- (void)remote:(id)sender {
    if(self.busy)return;
    [self run:self.engine args:@[@"remote-login"] input:nil done:^(BOOL ok,NSString *msg){
        if(!ok){[self error:msg];return;}
        NSAlert *a=[NSAlert new];a.messageText=@"远程连接";a.informativeText=msg;[a beginSheetModalForWindow:self.window completionHandler:nil];
    }];
}
- (void)inbox:(id)sender {
    NSURL *u=self.receiveDirURL;
    NSError *err=nil;
    if(![NSFileManager.defaultManager createDirectoryAtURL:u withIntermediateDirectories:YES attributes:nil error:&err]){[self error:err.localizedDescription?:@"无法打开收件箱"];return;}
    if(![NSWorkspace.sharedWorkspace openURL:u]) NSBeep();
}
- (void)openReceivedFolder:(id)sender {
    NSURL *file=self.files.selectedItem.representedObject;
    NSURL *folder=file?file.URLByDeletingLastPathComponent:self.receiveDirURL;
    if(folder && ![NSWorkspace.sharedWorkspace openURL:folder]) NSBeep();
}
- (void)scan {
    if(self.devices.hidden || self.busy || self.scanning || !self.engine)return;
    self.scanning=YES;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0),^{
        NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:self.engine];task.arguments=@[@"devices"];
        NSPipe *pipe=[NSPipe pipe];task.standardOutput=pipe;task.standardError=pipe;NSError *err=nil;
        NSArray *items=nil;NSString *failure=nil;
        if([task launchAndReturnError:&err]){NSData *data=[pipe.fileHandleForReading readDataToEndOfFile];[task waitUntilExit];
            if(task.terminationStatus==0)items=[NSJSONSerialization JSONObjectWithData:data options:0 error:nil];else failure=[[NSString alloc]initWithData:data encoding:NSUTF8StringEncoding];
        }else failure=err.localizedDescription;
        dispatch_async(dispatch_get_main_queue(),^{self.scanning=NO;
            if(![self.lastDevices isEqual:items]){self.lastDevices=items;NSString *prev=self.picker.selectedItem.representedObject;
                [self.picker removeAllItems];for(NSDictionary *p in items){NSMenuItem *item=[[NSMenuItem alloc]initWithTitle:p[@"Name"] action:nil keyEquivalent:@""];item.representedObject=p[@"ID"];[self.picker.menu addItem:item];}
                NSInteger idx=prev?[self.picker indexOfItemWithRepresentedObject:prev]:-1;if(self.picker.numberOfItems)[self.picker selectItemAtIndex:idx<0&&items.count==1?0:idx];}
            self.deviceHint.stringValue=failure?[failure stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]:(items.count?@"选择要双向配对的电脑":@"等待另一台 LANSend 登录同一账号…");});
    });
}
- (void)connectDevice:(id)sender {
    NSString *device=self.picker.selectedItem.representedObject;if(self.busy||!device)return;
    [self run:self.engine args:@[@"connect-device",device] input:nil done:^(BOOL ok,NSString *msg){
        if(!ok){[self error:msg];return;}[self refreshPeer];NSAlert *a=[NSAlert new];a.messageText=@"配对完成";a.informativeText=@"请在两端的设备管理中勾选启用对方。";[a beginSheetModalForWindow:self.window completionHandler:nil];}];
}
- (void)tab:(LSSegmentedBar*)c {
    self.transfer.hidden=c.selectedSegment!=0;
    self.settings.hidden=c.selectedSegment!=1;
    if (!self.settings.hidden) [self refreshPairedCount];
    self.devices.hidden=c.selectedSegment!=2;
    [self refreshReceiveDir];
    [self scan];
}
- (BOOL)windowShouldClose:(NSWindow*)sender {if(self.busy){NSBeep();return NO;}return YES;}
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication*)sender{return NO;}
- (void)applicationWillTerminate:(NSNotification*)notification {
    if (self.receiver.isRunning) [self.receiver terminate];
}
- (BOOL)applicationShouldHandleReopen:(NSApplication*)app hasVisibleWindows:(BOOL)visible {
    if (!self.window.visible) {
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [self.window makeKeyAndOrderFront:nil];
    }
    return YES;
}
- (void)windowWillClose:(NSNotification*)note {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
- (NSString*)silentSettingPath {
    return [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/silent-receipts"];
}
- (BOOL)silentReceipts {
    return [NSFileManager.defaultManager fileExistsAtPath:self.silentSettingPath];
}
- (void)changeMode:(LSSegmentedBar*)control {
    BOOL silent=control.selectedSegment==1;
    NSError *error=nil;
    if(silent) [[NSData data] writeToFile:self.silentSettingPath options:NSDataWritingAtomic error:&error];
    else if(self.silentReceipts) [NSFileManager.defaultManager removeItemAtPath:self.silentSettingPath error:&error];
    if(error){control.selectedSegment=self.silentReceipts?1:0;[self error:error.localizedDescription];return;}
    if(silent) [self.receipt close];
}
- (NSString*)defaultReceiveDirPath {
    return [NSHomeDirectory() stringByAppendingPathComponent:@"Downloads/LANSend"];
}
- (NSString*)receiveDirSettingPath {
    return [NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/receive-dir"];
}
- (NSString*)receiveDirPath {
    NSString *raw=[NSString stringWithContentsOfFile:self.receiveDirSettingPath encoding:NSUTF8StringEncoding error:nil];
    if(raw.length){
        if([raw hasPrefix:@"\uFEFF"]) raw=[raw substringFromIndex:1];
        NSString *trimmed=[raw stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet];
        if(trimmed.length && [trimmed hasPrefix:@"/"]) return [trimmed stringByStandardizingPath];
    }
    return self.defaultReceiveDirPath;
}
- (NSURL*)receiveDirURL {
    return [NSURL fileURLWithPath:self.receiveDirPath isDirectory:YES];
}
- (BOOL)isCustomReceiveDir {
    return ![self.receiveDirPath isEqualToString:[self.defaultReceiveDirPath stringByStandardizingPath]];
}
- (void)refreshReceiveDir {
    NSString *dir=self.receiveDirPath;
    if(self.receiveDirField){self.receiveDirField.stringValue=dir;self.receiveDirField.toolTip=dir;}
    if(self.resetDirButton) self.resetDirButton.enabled=self.isCustomReceiveDir;
}
- (BOOL)validateWritableDirectory:(NSString*)path error:(NSError**)outError {
    BOOL isDir=NO;
    if([NSFileManager.defaultManager fileExistsAtPath:path isDirectory:&isDir] && !isDir){
        if(outError) *outError=[NSError errorWithDomain:@"LANSend" code:1 userInfo:@{NSLocalizedDescriptionKey:@"所选路径不是文件夹"}];
        return NO;
    }
    NSError *err=nil;
    if(![NSFileManager.defaultManager createDirectoryAtPath:path withIntermediateDirectories:YES attributes:nil error:&err]){
        if(outError) *outError=[NSError errorWithDomain:@"LANSend" code:2 userInfo:@{NSLocalizedDescriptionKey:@"所选目录无法写入，请检查权限或更换文件夹。"}];
        return NO;
    }
    NSString *probe=[path stringByAppendingPathComponent:[NSString stringWithFormat:@".lansend-check-%@",NSUUID.UUID.UUIDString]];
    if(![[NSData data] writeToFile:probe options:NSDataWritingAtomic error:&err]){
        if(outError) *outError=[NSError errorWithDomain:@"LANSend" code:3 userInfo:@{NSLocalizedDescriptionKey:@"所选目录无法写入，请检查权限或更换文件夹。"}];
        return NO;
    }
    [NSFileManager.defaultManager removeItemAtPath:probe error:nil];
    return YES;
}
- (void)chooseReceiveDir:(id)sender {
    if(self.busy) return;
    NSOpenPanel *p=NSOpenPanel.openPanel;
    p.canChooseFiles=NO; p.canChooseDirectories=YES; p.allowsMultipleSelection=NO; p.canCreateDirectories=YES;
    p.prompt=@"选择"; p.directoryURL=self.receiveDirURL;
    [p beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse result){
        if(result!=NSModalResponseOK || !p.URL) return;
        NSString *chosen=[p.URL.path stringByStandardizingPath];
        NSError *err=nil;
        if(![self validateWritableDirectory:chosen error:&err]){
            [self error:err.localizedDescription?:@"所选目录无法写入，请检查权限或更换文件夹。"];
            return;
        }
        NSString *def=[self.defaultReceiveDirPath stringByStandardizingPath];
        if([chosen isEqualToString:def]){
            if([NSFileManager.defaultManager fileExistsAtPath:self.receiveDirSettingPath] &&
               ![NSFileManager.defaultManager removeItemAtPath:self.receiveDirSettingPath error:&err]){
                [self error:err.localizedDescription];
                return;
            }
        }else{
            NSString *parent=[self.receiveDirSettingPath stringByDeletingLastPathComponent];
            if(![NSFileManager.defaultManager createDirectoryAtPath:parent withIntermediateDirectories:YES attributes:nil error:&err] ||
               ![[chosen stringByAppendingString:@"\n"] writeToFile:self.receiveDirSettingPath atomically:YES encoding:NSUTF8StringEncoding error:&err]){
                [self error:err.localizedDescription];
                return;
            }
        }
        [self refreshReceiveDir];
        if(!self.receiver.isRunning) [self startReceiver];
    }];
}
- (void)resetReceiveDir:(id)sender {
    if(self.busy) return;
    NSError *err=nil;
    if([NSFileManager.defaultManager fileExistsAtPath:self.receiveDirSettingPath] &&
       ![NSFileManager.defaultManager removeItemAtPath:self.receiveDirSettingPath error:&err]){
        [self error:err.localizedDescription];
        return;
    }
    [NSFileManager.defaultManager createDirectoryAtURL:self.receiveDirURL withIntermediateDirectories:YES attributes:nil error:nil];
    [self refreshReceiveDir];
    if(!self.receiver.isRunning) [self startReceiver];
}
- (void)openReceived:(id)sender {
    NSURL *file=self.files.selectedItem.representedObject;
    if(file && ![NSWorkspace.sharedWorkspace openURL:file]) NSBeep();
}
- (void)showReceived:(NSString*)name {
    if(self.silentReceipts || !name.length ||
       ![name.lastPathComponent isEqual:name] || [name isEqual:@"."] || [name isEqual:@".."]) return;
    if(!self.receipt){
        self.receipt=[[NSPanel alloc] initWithContentRect:NSMakeRect(0,0,430,184)
            styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskFullSizeContentView
            backing:NSBackingStoreBuffered defer:NO];
        self.receipt.title=@"LANSend 收件";
        self.receipt.titleVisibility=NSWindowTitleHidden;
        self.receipt.titlebarAppearsTransparent=YES;

        self.receipt.backgroundColor=LSCardColor();
        self.receipt.releasedWhenClosed=NO;
        self.receipt.floatingPanel=YES; self.receipt.hidesOnDeactivate=NO;
        self.receipt.collectionBehavior=NSWindowCollectionBehaviorMoveToActiveSpace|NSWindowCollectionBehaviorFullScreenAuxiliary;
        NSView *v=self.receipt.contentView;
        self.receiptTitle=[self label:@"" frame:NSMakeRect(20,116,390,26) size:16 bold:YES color:LSInkPrimary() view:v];
        self.files=[[NSPopUpButton alloc] initWithFrame:NSMakeRect(20,70,390,32) pullsDown:NO];
        [v addSubview:self.files];
        [self button:@"打开文件" at:NSMakeRect(20,20,116,34) action:@selector(openReceived:) primary:YES view:v];
        [self button:@"打开收件夹" at:NSMakeRect(146,20,120,34) action:@selector(openReceivedFolder:) primary:NO view:v];
        [self.receipt center];
    }
    BOOL first=!self.receipt.visible;
    if(first) [self.files removeAllItems];
    NSURL *folder=self.receiveDirURL;
    NSMenuItem *item=[[NSMenuItem alloc] initWithTitle:name action:nil keyEquivalent:@""];
    item.representedObject=[folder URLByAppendingPathComponent:name];
    [self.files.menu addItem:item];
    if(first) [self.files selectItem:item];
    self.receiptTitle.stringValue=[NSString stringWithFormat:@"已收到 %ld 个文件",(long)self.files.numberOfItems];
    if(first){[NSApp activateIgnoringOtherApps:YES];[self.receipt makeKeyAndOrderFront:nil];}
}
- (void)readEvents:(NSFileHandle*)handle {
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0), ^{
        NSMutableData *buffer=[NSMutableData data];
        for(NSData *part; (part=handle.availableData).length; ){
            [buffer appendData:part];
            NSRange line;
            while((line=[buffer rangeOfData:[NSData dataWithBytes:"\n" length:1] options:0 range:NSMakeRange(0,buffer.length)]).location!=NSNotFound){
                NSString *record=[[NSString alloc] initWithData:[buffer subdataWithRange:NSMakeRange(0,line.location)] encoding:NSUTF8StringEncoding];
                [buffer replaceBytesInRange:NSMakeRange(0,NSMaxRange(line)) withBytes:NULL length:0];
                if(![record hasPrefix:@"RECEIVED "]) continue;
                NSData *data=[[NSData alloc] initWithBase64EncodedString:[record substringFromIndex:9] options:0];
                NSString *name=[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
                dispatch_async(dispatch_get_main_queue(), ^{[self showReceived:name];});
            }
        }
    });
}
- (void)startReceiver {
    NSURL *dest=self.receiveDirURL;
    NSError *accessError=nil;
    [NSFileManager.defaultManager createDirectoryAtURL:dest withIntermediateDirectories:YES attributes:nil error:&accessError];
    if(!accessError) [NSFileManager.defaultManager contentsOfDirectoryAtURL:dest includingPropertiesForKeys:nil options:NSDirectoryEnumerationSkipsHiddenFiles error:&accessError];
    if (accessError) {
        if (self.isCustomReceiveDir) {
            self.status.stringValue=@"自定义收件目录不可访问";
            if(self.window.visible) [self error:@"当前自定义收件目录不可访问，请在「配对与设置」中重新选择或恢复默认目录。"];
        } else {
            self.status.stringValue=@"接收目录不可访问";
            if(self.window.visible) [self error:@"请允许 LANSend 访问「下载」文件夹，然后重新打开 LANSend。"];
        }
        return;
    }
    NSTask *task=[NSTask new];
    task.executableURL=[NSURL fileURLWithPath:self.engine];
    task.arguments=@[@"receive"];
    self.events=[NSPipe pipe];
    task.standardOutput=self.events;
    task.standardError=[NSFileHandle fileHandleWithNullDevice];
    NSError *launchError=nil;
    if (![task launchAndReturnError:&launchError]) {
        self.status.stringValue=@"接收服务启动失败";
        if(self.window.visible) [self error:launchError.localizedDescription];
        return;
    }
    self.receiver=task;
    [self readEvents:self.events.fileHandleForReading];
    [self run:self.engine args:@[@"ready"] input:nil done:^(BOOL ok,NSString *msg){
        self.status.stringValue=ok && task.isRunning ? @"接收服务已就绪" : @"接收服务启动失败";
        if((!ok || !task.isRunning) && self.window.visible) [self error:msg.length ? msg : @"后台接收服务未运行"];
    }];
}
- (void)applicationDidFinishLaunching:(NSNotification*)note {
    NSMenu *main=[NSMenu new];NSMenuItem *appItem=[NSMenuItem new];[main addItem:appItem];NSMenu *appMenu=[NSMenu new];appItem.submenu=appMenu;
    [appMenu addItemWithTitle:@"退出 LANSend" action:@selector(terminate:) keyEquivalent:@"q"];
    NSMenuItem *edit=[NSMenuItem new];[main addItem:edit];edit.submenu=[[NSMenu alloc]initWithTitle:@"编辑"];
    for(NSArray *a in @[@[@"剪切",@"cut:",@"x"],@[@"复制",@"copy:",@"c"],@[@"粘贴",@"paste:",@"v"],@[@"全选",@"selectAll:",@"a"]])[edit.submenu addItemWithTitle:a[0] action:NSSelectorFromString(a[1]) keyEquivalent:a[2]];
    NSApp.mainMenu=main;

    self.window=[[NSWindow alloc] initWithContentRect:NSMakeRect(0,0,620,588)
        styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskMiniaturizable|NSWindowStyleMaskFullSizeContentView
        backing:NSBackingStoreBuffered defer:NO];
    self.window.title=@"LANSend";
    self.window.titleVisibility=NSWindowTitleHidden;
    self.window.titlebarAppearsTransparent=YES;

    self.window.backgroundColor=LSCanvasColor();
    self.window.delegate=self;
    self.window.releasedWhenClosed=NO;

    NSView *v=self.window.contentView;
    v.wantsLayer=YES;


    LSSurface *header=[[LSSurface alloc] initWithFrame:NSMakeRect(0,520,620,68)];
    header.wantsLayer=YES;
    header.fill=LSCardColor();
    [v addSubview:header];
    [self label:@"LANSend" frame:NSMakeRect(24,14,180,26) size:19 bold:YES color:LSInkPrimary() view:header];
    self.peerBadge=[[LSStatusBadge alloc] initWithFrame:NSMakeRect(340,13,256,28)];
    self.peerBadge.connectedCount=-1;
    [header addSubview:self.peerBadge];
    [self refreshPeer];

    // 1px hairline header divider
    LSSurface *divider=[[LSSurface alloc] initWithFrame:NSMakeRect(0,519,620,1)];
    divider.wantsLayer=YES;
    divider.fill=LSDividerColor();
    [v addSubview:divider];

    // Segmented navigation bar
    LSSegmentedBar *tabs=[[LSSegmentedBar alloc] initWithFrame:NSMakeRect(24,467,324,36)
        labels:@[@"文件传输",@"配对与设置",@"我的电脑"] target:self action:@selector(tab:)];
    tabs.selectedSegment=0;
    [v addSubview:tabs];

    NSRect pageFrame=NSMakeRect(24,20,572,433);
    self.queue=[NSMutableArray array];

    // Page 0: 文件传输
    TransferDropView *drop=[[TransferDropView alloc] initWithFrame:pageFrame];
    [drop registerForDraggedTypes:@[NSPasteboardTypeFileURL]];
    __weak Desktop *weakSelf=self;
    drop.enqueue=^(NSArray<NSURL *> *urls){[weakSelf enqueueURLs:urls];};
    self.transfer=drop;
    [v addSubview:self.transfer];

    LSCardView *sendCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,293,572,140)];
    drop.dropHighlightCard=sendCard;
    [self.transfer addSubview:sendCard];
    [self label:@"发送文件" frame:NSMakeRect(20,96,532,26) size:17 bold:YES color:LSInkPrimary() view:sendCard];
    [self label:@"将文件或文件夹拖入窗口任意位置，或点击下方按钮选择。" frame:NSMakeRect(20,70,532,20) size:12.5 bold:NO color:LSInkSecondary() view:sendCard];
    self.sendButton=[self button:@"选择文件或文件夹…" at:NSMakeRect(20,20,168,36) action:@selector(send:) primary:YES view:sendCard];

    LSCardView *queueCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,50,572,229)];
    [self.transfer addSubview:queueCard];
    [self label:@"传输队列" frame:NSMakeRect(20,191,200,22) size:13.5 bold:YES color:LSInkPrimary() view:queueCard];
    self.status=[self label:@"正在启动接收服务…" frame:NSMakeRect(230,191,322,22) size:12 bold:NO color:LSInkSecondary() view:queueCard];
    self.status.alignment=NSTextAlignmentRight;

    LSCardView *queueFrame=[[LSCardView alloc] initWithFrame:NSMakeRect(20,38,532,142)];
    queueFrame.wantsLayer=YES;
    queueFrame.layer.cornerRadius=8.0;
    queueFrame.layer.borderWidth=1.0;
    queueFrame.layer.masksToBounds=YES;
    [queueCard addSubview:queueFrame];

    NSScrollView *scroll=[[NSScrollView alloc] initWithFrame:NSMakeRect(2,2,528,138)];
    scroll.hasVerticalScroller=YES;
    scroll.borderType=NSNoBorder;
    scroll.drawsBackground=NO;
    self.queueView=[[NSTextView alloc] initWithFrame:scroll.contentView.bounds];
    self.queueView.editable=NO;
    self.queueView.drawsBackground=NO;
    self.queueView.font=[NSFont systemFontOfSize:12.5 weight:NSFontWeightRegular];
    self.queueView.textColor=LSInkPrimary();
    self.queueView.textContainerInset=NSMakeSize(8,8);
    self.queueView.autoresizingMask=NSViewWidthSizable;
    scroll.documentView=self.queueView;
    [queueFrame addSubview:scroll];

    self.progress=[[NSProgressIndicator alloc] initWithFrame:NSMakeRect(20,16,532,8)];
    self.progress.indeterminate=YES;
    self.progress.displayedWhenStopped=NO;
    [queueCard addSubview:self.progress];

    self.retryButton=[self button:@"重试失败项" at:NSMakeRect(0,0,110,34) action:@selector(retryFailed:) primary:NO view:self.transfer];
    self.retryButton.enabled=NO;
    [self button:@"打开收件箱" at:NSMakeRect(458,0,114,34) action:@selector(inbox:) primary:NO view:self.transfer];

    // Page 1: 配对与设置
    self.settings=[[NSView alloc] initWithFrame:pageFrame];
    self.settings.hidden=YES;
    [v addSubview:self.settings];

    LSCardView *deviceInfoCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,309,572,124)];
    [self.settings addSubview:deviceInfoCard];
    [self label:@"设备信息" frame:NSMakeRect(20,88,532,20) size:13.5 bold:YES color:LSInkPrimary() view:deviceInfoCard];
    [self label:@"本机名称" frame:NSMakeRect(20,50,66,20) size:13 bold:NO color:LSInkSecondary() view:deviceInfoCard];
    NSString *namePath=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/device-name"];
    NSString *name=[NSString stringWithContentsOfFile:namePath encoding:NSUTF8StringEncoding error:nil]?:NSProcessInfo.processInfo.hostName;
    self.nameLabel=[self label:name frame:NSMakeRect(102,50,345,20) size:13 bold:NO color:LSInkPrimary() view:deviceInfoCard];
    self.nameField=[self styledFieldWithFrame:NSMakeRect(102,47,265,26) editable:YES view:deviceInfoCard];
    self.nameField.delegate=self;
    self.nameField.target=self;
    self.nameField.action=@selector(saveDeviceName:);
    self.nameField.hidden=YES;
    self.cancelNameButton=[self button:@"取消" at:NSMakeRect(376,44,82,32) action:@selector(cancelNameEdit:) primary:NO view:deviceInfoCard];
    self.cancelNameButton.hidden=YES;
    self.editNameButton=[self button:@"编辑" at:NSMakeRect(466,44,86,32) action:@selector(saveDeviceName:) primary:NO view:deviceInfoCard];
    [self label:@"已配对设备" frame:NSMakeRect(20,15,80,20) size:13 bold:NO color:LSInkSecondary() view:deviceInfoCard];
    self.pairedCount=[self label:@"0 台" frame:NSMakeRect(102,15,220,20) size:13 bold:NO color:LSInkPrimary() view:deviceInfoCard];
    [self button:@"管理…" at:NSMakeRect(466,9,86,32) action:@selector(editNickname:) primary:NO view:deviceInfoCard];

    LSCardView *lanPairCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,163,572,132)];
    [self.settings addSubview:lanPairCard];
    [self label:@"局域网配对" frame:NSMakeRect(20,96,532,20) size:13.5 bold:YES color:LSInkPrimary() view:lanPairCard];
    [self label:@"在一台电脑复制 LANSend 码，在另一台电脑粘贴即可双向配对。" frame:NSMakeRect(20,70,532,20) size:12.5 bold:NO color:LSInkSecondary() view:lanPairCard];
    self.pairButton=[self button:@"从剪贴板配对" at:NSMakeRect(20,20,136,34) action:@selector(pasteInvite:) primary:YES view:lanPairCard];
    [self button:@"复制 LANSend 码" at:NSMakeRect(166,20,144,34) action:@selector(copyInvite:) primary:NO view:lanPairCard];

    LSCardView *receiveCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,9,572,140)];
    [self.settings addSubview:receiveCard];
    [self label:@"接收设置" frame:NSMakeRect(20,104,532,20) size:13.5 bold:YES color:LSInkPrimary() view:receiveCard];
    [self label:@"接收方式" frame:NSMakeRect(20,66,66,20) size:13 bold:NO color:LSInkSecondary() view:receiveCard];
    LSSegmentedBar *mode=[[LSSegmentedBar alloc] initWithFrame:NSMakeRect(92,60,184,32)
        labels:@[@"前台弹窗",@"静默接收"] target:self action:@selector(changeMode:)];
    mode.selectedSegment=self.silentReceipts?1:0;
    [receiveCard addSubview:mode];
    [self label:@"收件目录" frame:NSMakeRect(20,22,66,20) size:13 bold:NO color:LSInkSecondary() view:receiveCard];
    self.receiveDirField=[self label:@"" frame:NSMakeRect(92,22,284,20) size:13 bold:NO color:LSInkPrimary() view:receiveCard];
    self.receiveDirField.selectable=YES;
    [self button:@"更改…" at:NSMakeRect(386,16,72,32) action:@selector(chooseReceiveDir:) primary:NO view:receiveCard];
    self.resetDirButton=[self button:@"恢复默认" at:NSMakeRect(466,16,86,32) action:@selector(resetReceiveDir:) primary:NO view:receiveCard];
    [self refreshReceiveDir];

    // Page 2: 我的电脑
    self.devices=[[NSView alloc] initWithFrame:pageFrame];
    self.devices.hidden=YES;
    [v addSubview:self.devices];

    LSCardView *remoteCard=[[LSCardView alloc] initWithFrame:NSMakeRect(0,205,572,228)];
    [self.devices addSubview:remoteCard];
    [self label:@"跨网络同账号设备" frame:NSMakeRect(20,184,532,26) size:17 bold:YES color:LSInkPrimary() view:remoteCard];
    [self label:@"在线设备" frame:NSMakeRect(20,154,532,20) size:13 bold:YES color:LSInkPrimary() view:remoteCard];
    self.deviceHint=[self label:@"正在查找同一账号的电脑…" frame:NSMakeRect(20,114,532,34) size:12.5 bold:NO color:LSInkSecondary() view:remoteCard];
    self.deviceHint.lineBreakMode=NSLineBreakByWordWrapping;
    self.deviceHint.maximumNumberOfLines=2;
    self.picker=[[NSPopUpButton alloc] initWithFrame:NSMakeRect(20,70,532,32) pullsDown:NO];
    [remoteCard addSubview:self.picker];
    [self button:@"配对所选电脑" at:NSMakeRect(20,20,136,34) action:@selector(connectDevice:) primary:YES view:remoteCard];
    [self button:@"远程连接…" at:NSMakeRect(166,20,116,34) action:@selector(remote:) primary:NO view:remoteCard];

    self.scanTimer=[NSTimer scheduledTimerWithTimeInterval:5 repeats:YES block:^(NSTimer *timer){[self refreshPeer];[self scan];}];
    [self.window center];
    if (!self.pendingOpen.count && [NSProcessInfo.processInfo.arguments containsObject:@"--background-receiver"]) {
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    } else {
        [self.window makeKeyAndOrderFront:nil]; [NSApp activateIgnoringOtherApps:YES];
    }
    NSString *res=NSBundle.mainBundle.resourcePath;self.engine=[res stringByAppendingPathComponent:@"engine"];[self refreshPeer];
    [self run:@"/bin/bash" args:@[[res stringByAppendingPathComponent:@"Setup.sh"]] input:nil done:^(BOOL ok,NSString *msg){
        if(ok){self.readyToSend=YES;[self startReceiver];}else{self.status.stringValue=@"接收服务启动失败";if(self.window.visible)[self error:msg];}
    }];
}
@end
int main(void){@autoreleasepool{NSApplication *app=NSApplication.sharedApplication;Desktop * __attribute__((objc_precise_lifetime)) delegate=[Desktop new];app.delegate=delegate;[app run];}return 0;}
