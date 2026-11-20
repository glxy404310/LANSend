#import <Cocoa/Cocoa.h>

@interface Desktop:NSObject <NSApplicationDelegate,NSWindowDelegate>
@property NSWindow *window;
@property NSTextField *peer,*status,*receiveDirField;
@property NSButton *sendButton,*pairButton,*resetDirButton;
@property NSProgressIndicator *progress;
@property NSView *transfer,*settings,*devices;
@property NSPopUpButton *picker;
@property NSTextField *deviceHint;
@property BOOL scanning;
@property NSArray *lastDevices;
@property NSTimer *scanTimer;
@property BOOL busy;
@property NSString *engine;
@property NSTask *receiver;
@property NSPanel *receipt;
@property NSPopUpButton *files;
@property NSTextField *receiptTitle;
@property NSPipe *events;
@end
@implementation Desktop
- (NSTextField*)label:(NSString*)value frame:(NSRect)rect size:(CGFloat)size bold:(BOOL)bold view:(NSView*)parent {
    NSTextField *f=[NSTextField labelWithString:value]; f.frame=rect;
    f.font=[NSFont systemFontOfSize:size weight:bold?NSFontWeightSemibold:NSFontWeightRegular];
    f.lineBreakMode=NSLineBreakByTruncatingMiddle; [parent addSubview:f];return f;
}
- (NSButton*)button:(NSString*)title at:(NSRect)rect action:(SEL)action view:(NSView*)parent {
    NSButton *b=[NSButton buttonWithTitle:title target:self action:action];b.frame=rect;[parent addSubview:b];return b;
}
- (void)refreshPeer {
    NSString *path=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/peer.json"];
    NSData *data=[NSData dataWithContentsOfFile:path];NSDictionary *c=data?[NSJSONSerialization JSONObjectWithData:data options:0 error:nil]:nil;
    self.peer.stringValue=[c[@"PeerHash"] length]?@"已配对":@"尚未配对";
}
- (void)run:(NSString*)program args:(NSArray*)args input:(NSString*)input done:(void(^)(BOOL,NSString*))done {
    self.busy=YES;self.sendButton.enabled=NO;self.pairButton.enabled=NO;
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
                if([args.firstObject isEqual:@"send"]){NSString *s=[[NSString alloc]initWithData:part encoding:NSUTF8StringEncoding];
                    NSString *line=[[s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet].lastObject;
                    if(line.length)dispatch_async(dispatch_get_main_queue(),^{self.status.stringValue=line;});
                }
            }
            [task waitUntilExit];ok=task.terminationStatus==0;
        }
        NSString *message=e.localizedDescription?:([[NSString alloc]initWithData:tail encoding:NSUTF8StringEncoding]?:@"");
        dispatch_async(dispatch_get_main_queue(),^{self.busy=NO;self.sendButton.enabled=YES;self.pairButton.enabled=YES;[self.progress stopAnimation:nil];done(ok,[message stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]);});
    });
}
- (void)error:(NSString*)msg {NSAlert *a=[NSAlert new];a.messageText=@"操作未完成";a.informativeText=msg;[a beginSheetModalForWindow:self.window completionHandler:nil];}
- (void)send:(id)sender {
    NSOpenPanel *p=NSOpenPanel.openPanel;p.allowsMultipleSelection=YES;p.canChooseDirectories=NO;p.prompt=@"发送";
    [p beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse result){
        if(result!=NSModalResponseOK)return;
        NSMutableArray *args=[NSMutableArray arrayWithObject:@"send"];for(NSURL *u in p.URLs)[args addObject:u.path];
        self.status.stringValue=@"正在发送…";[self.progress startAnimation:nil];
        [self run:self.engine args:args input:nil done:^(BOOL ok,NSString *msg){
            self.status.stringValue=ok?[NSString stringWithFormat:@"已发送 %lu 个文件",(unsigned long)(args.count-1)]:@"发送未完成";if(!ok)[self error:[msg componentsSeparatedByString:@"\n"].lastObject];
        }];
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
    if(expected.length){
        NSAlert *confirm=[NSAlert new];confirm.messageText=@"已有配对";
        confirm.informativeText=@"本机已有配对。是否允许在本次配对成功后更换设备？";
        [confirm addButtonWithTitle:@"取消"];[confirm addButtonWithTitle:@"允许更换"];
        if([confirm runModal]!=NSAlertSecondButtonReturn)return;
    }
    [self run:self.engine args:@[copy?@"invite-new":@"invite-join",expected] input:input done:^(BOOL ok,NSString *msg){
        if(!ok){[self error:msg];return;}
        if(copy){
            [NSPasteboard.generalPasteboard clearContents];
            if(![NSPasteboard.generalPasteboard setString:msg forType:NSPasteboardTypeString]){[self error:@"复制配对码失败"];return;}
        }
        [self refreshPeer];
        NSAlert *alert=[NSAlert new];alert.messageText=copy?@"已复制 LANSend 码":@"已双向配对";
        alert.informativeText=copy?@"请在另一台电脑点击「从剪贴板配对」。配对码 5 分钟内有效。":@"两台电脑现在可以互传文件。";
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
 if(!ok){[self error:msg];return;}[self refreshPeer];NSAlert *a=[NSAlert new];a.messageText=@"配对完成";a.informativeText=@"两台电脑现在可以互传文件。";[a beginSheetModalForWindow:self.window completionHandler:nil];}];
}
- (void)tab:(NSSegmentedControl*)c {self.transfer.hidden=c.selectedSegment!=0;self.settings.hidden=c.selectedSegment!=1;self.devices.hidden=c.selectedSegment!=2;[self refreshReceiveDir];[self scan];}
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
- (void)changeMode:(NSSegmentedControl*)control {
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
        self.receipt=[[NSPanel alloc] initWithContentRect:NSMakeRect(0,0,420,152)
            styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable backing:NSBackingStoreBuffered defer:NO];
        self.receipt.title=@"LANSend 收件"; self.receipt.releasedWhenClosed=NO;
        self.receipt.floatingPanel=YES; self.receipt.hidesOnDeactivate=NO;
        self.receipt.collectionBehavior=NSWindowCollectionBehaviorMoveToActiveSpace|NSWindowCollectionBehaviorFullScreenAuxiliary;
        NSView *v=self.receipt.contentView;
        self.receiptTitle=[self label:@"" frame:NSMakeRect(20,104,380,30) size:20 bold:YES view:v];
        self.files=[[NSPopUpButton alloc] initWithFrame:NSMakeRect(20,60,380,32) pullsDown:NO];
        [v addSubview:self.files];
        [self button:@"打开文件" at:NSMakeRect(20,14,116,34) action:@selector(openReceived:) view:v];
        [self button:@"打开收件夹" at:NSMakeRect(144,14,124,34) action:@selector(openReceivedFolder:) view:v];
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
    self.window=[[NSWindow alloc]initWithContentRect:NSMakeRect(0,0,580,458) styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskMiniaturizable backing:NSBackingStoreBuffered defer:NO];
    self.window.title=@"LANSend";self.window.delegate=self;self.window.releasedWhenClosed=NO;
    NSView *v=self.window.contentView;NSView *header=[[NSView alloc]initWithFrame:NSMakeRect(0,352,580,106)];header.wantsLayer=YES;header.layer.backgroundColor=[NSColor colorWithSRGBRed:.075 green:.208 blue:.239 alpha:1].CGColor;[v addSubview:header];
    [self label:@"LANSend" frame:NSMakeRect(24,48,532,39) size:30 bold:YES view:header].textColor=NSColor.whiteColor;
    self.peer=[self label:@"" frame:NSMakeRect(24,20,532,22) size:14 bold:NO view:header];self.peer.textColor=NSColor.whiteColor;[self refreshPeer];
    NSSegmentedControl *tabs=[NSSegmentedControl segmentedControlWithLabels:@[@"文件传输",@"配对与设置",@"我的电脑"] trackingMode:NSSegmentSwitchTrackingSelectOne target:self action:@selector(tab:)];tabs.frame=NSMakeRect(24,306,324,30);tabs.selectedSegment=0;[v addSubview:tabs];
    self.transfer=[[NSView alloc]initWithFrame:NSMakeRect(24,16,532,280)];[v addSubview:self.transfer];self.settings=[[NSView alloc]initWithFrame:self.transfer.frame];[v addSubview:self.settings];self.settings.hidden=YES;
    [self label:@"发送到已配对设备" frame:NSMakeRect(0,226,532,32) size:22 bold:YES view:self.transfer];
    self.sendButton=[self button:@"选择文件并发送…" at:NSMakeRect(0,168,168,36) action:@selector(send:) view:self.transfer];self.sendButton.bezelColor=[NSColor colorWithSRGBRed:.1 green:.46 blue:.41 alpha:1];
    [self button:@"打开收件箱" at:NSMakeRect(178,168,120,36) action:@selector(inbox:) view:self.transfer];
    self.status=[self label:@"正在启动接收服务…" frame:NSMakeRect(0,34,532,22) size:14 bold:NO view:self.transfer];
    self.progress=[[NSProgressIndicator alloc]initWithFrame:NSMakeRect(0,16,532,8)];self.progress.indeterminate=YES;self.progress.displayedWhenStopped=NO;[self.transfer addSubview:self.progress];
    [self label:@"局域网配对" frame:NSMakeRect(0,242,532,32) size:20 bold:YES view:self.settings];
    [self label:@"在一台电脑复制 LANSend 码，在另一台电脑粘贴即可双向配对。" frame:NSMakeRect(0,210,532,24) size:14 bold:NO view:self.settings];
    [self button:@"复制 LANSend 码" at:NSMakeRect(0,158,148,36) action:@selector(copyInvite:) view:self.settings];
    self.pairButton=[self button:@"从剪贴板配对" at:NSMakeRect(158,158,140,36) action:@selector(pasteInvite:) view:self.settings];
    NSSegmentedControl *mode=[NSSegmentedControl segmentedControlWithLabels:@[@"前台弹窗",@"静默接收"] trackingMode:NSSegmentSwitchTrackingSelectOne target:self action:@selector(changeMode:)];
    mode.frame=NSMakeRect(324,98,208,32);mode.selectedSegment=self.silentReceipts?1:0;
    [self.settings addSubview:mode];
    NSBox *sep=[[NSBox alloc]initWithFrame:NSMakeRect(0,78,532,1)];sep.boxType=NSBoxSeparator;[self.settings addSubview:sep];
    [self label:@"收件目录" frame:NSMakeRect(0,48,120,20) size:13 bold:NO view:self.settings];
    self.receiveDirField=[NSTextField new];self.receiveDirField.frame=NSMakeRect(0,16,348,24);self.receiveDirField.editable=NO;self.receiveDirField.selectable=YES;self.receiveDirField.bezeled=YES;self.receiveDirField.lineBreakMode=NSLineBreakByTruncatingMiddle;[self.settings addSubview:self.receiveDirField];
    [self button:@"更改…" at:NSMakeRect(356,12,80,32) action:@selector(chooseReceiveDir:) view:self.settings];
    self.resetDirButton=[self button:@"恢复默认" at:NSMakeRect(444,12,88,32) action:@selector(resetReceiveDir:) view:self.settings];
    [self refreshReceiveDir];
    self.devices=[[NSView alloc]initWithFrame:self.transfer.frame];[v addSubview:self.devices];self.devices.hidden=YES;
 self.deviceHint=[self label:@"正在查找同一账号的电脑…" frame:NSMakeRect(0,224,532,26) size:14 bold:NO view:self.devices];
 self.picker=[[NSPopUpButton alloc]initWithFrame:NSMakeRect(0,172,532,34) pullsDown:NO];[self.devices addSubview:self.picker];
 [self button:@"配对所选电脑" at:NSMakeRect(0,116,152,36) action:@selector(connectDevice:) view:self.devices];
 [self button:@"远程连接…" at:NSMakeRect(162,116,118,36) action:@selector(remote:) view:self.devices];
 self.scanTimer=[NSTimer scheduledTimerWithTimeInterval:5 repeats:YES block:^(NSTimer *timer){[self refreshPeer];[self scan];}];
 [self.window center];
    if ([NSProcessInfo.processInfo.arguments containsObject:@"--background-receiver"]) {
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    } else {
        [self.window makeKeyAndOrderFront:nil]; [NSApp activateIgnoringOtherApps:YES];
    }
    NSString *res=NSBundle.mainBundle.resourcePath;self.engine=[res stringByAppendingPathComponent:@"engine"];
    [self run:@"/bin/bash" args:@[[res stringByAppendingPathComponent:@"Setup.sh"]] input:nil done:^(BOOL ok,NSString *msg){
        if(ok){[self startReceiver];}else{self.status.stringValue=@"接收服务启动失败";if(self.window.visible)[self error:msg];}
    }];
}
@end
int main(void){@autoreleasepool{NSApplication *app=NSApplication.sharedApplication;Desktop * __attribute__((objc_precise_lifetime)) delegate=[Desktop new];app.delegate=delegate;[app run];}return 0;}
