#import <Cocoa/Cocoa.h>
#include "systray.h"

#if __MAC_OS_X_VERSION_MIN_REQUIRED < 101400

    #ifndef NSControlStateValueOff
      #define NSControlStateValueOff NSOffState
    #endif

    #ifndef NSControlStateValueOn
      #define NSControlStateValueOn NSOnState
    #endif

#endif

@interface MenuItem : NSObject
{
  @public
    NSNumber* menuId;
    NSNumber* parentMenuId;
    NSString* title;
    NSString* tooltip;
    short disabled;
    short checked;
}
-(id) initWithId: (int)theMenuId
withParentMenuId: (int)theParentMenuId
       withTitle: (const char*)theTitle
     withTooltip: (const char*)theTooltip
    withDisabled: (short)theDisabled
     withChecked: (short)theChecked;
     @end
     @implementation MenuItem
     -(id) initWithId: (int)theMenuId
     withParentMenuId: (int)theParentMenuId
            withTitle: (const char*)theTitle
          withTooltip: (const char*)theTooltip
         withDisabled: (short)theDisabled
          withChecked: (short)theChecked
{
  menuId = [NSNumber numberWithInt:theMenuId];
  parentMenuId = [NSNumber numberWithInt:theParentMenuId];
  title = [[NSString alloc] initWithCString:theTitle
                                   encoding:NSUTF8StringEncoding];
  tooltip = [[NSString alloc] initWithCString:theTooltip
                                     encoding:NSUTF8StringEncoding];
  disabled = theDisabled;
  checked = theChecked;
  return self;
}
@end

// SystrayController owns the NSStatusItem and menu WITHOUT becoming the
// NSApplicationDelegate. The hosting framework (Wails on macOS) installs its
// own delegate to drive the standard app lifecycle (applicationShouldTerminate:,
// OnBeforeClose, OnShutdown, Dock right-click "Quit" menu, ...). Overwriting
// that delegate — which this file used to do — broke the lifecycle: Dock /
// Cmd+Q would expose only "Force Quit" and graceful cleanup never ran.
@interface SystrayController : NSObject
- (void) setupOnMainThread;
- (IBAction) menuHandler:(id)sender;
- (void) setIcon:(NSImage *)image;
- (void) setTitle:(NSString *)title;
- (void) setTooltip:(NSString *)tooltip;
- (void) add_or_update_menu_item:(MenuItem*) item;
- (void) add_separator:(NSNumber*) menuId;
- (void) hide_menu_item:(NSNumber*) menuId;
- (void) show_menu_item:(NSNumber*) menuId;
- (void) setMenuItemIcon:(NSArray*) imageAndMenuId;
- (void) quit;
@end

static SystrayController *gSystrayController = nil;
static id gSystrayTerminateObserver = nil;
static NSStatusItem *gStatusItem = nil;
static NSMenu *gMenu = nil;

NSMenuItem *find_menu_item(NSMenu *ourMenu, NSNumber *menuId) {
  NSMenuItem *foundItem = [ourMenu itemWithTag:[menuId integerValue]];
  if (foundItem != NULL) {
    return foundItem;
  }
  NSArray *menu_items = ourMenu.itemArray;
  for (NSUInteger i = 0; i < [menu_items count]; i++) {
    NSMenuItem *i_item = [menu_items objectAtIndex:i];
    if (i_item.hasSubmenu) {
      foundItem = find_menu_item(i_item.submenu, menuId);
      if (foundItem != NULL) {
        return foundItem;
      }
    }
  }
  return NULL;
}

@implementation SystrayController

- (void)setupOnMainThread {
  if (gStatusItem == nil) {
    gStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
  }
  if (gMenu == nil) {
    gMenu = [[NSMenu alloc] init];
    [gMenu setAutoenablesItems:FALSE];
    [gStatusItem setMenu:gMenu];
  }
  // Show a text label immediately so the item is visible in the menu bar even
  // before Go's onReady sets the PNG icon (and on Sonoma+ when icons are hidden
  // in the Control Center overflow, "RV" still helps users find the app).
  gStatusItem.button.title = @"RV";
  if (@available(macOS 10.12, *)) {
    gStatusItem.visible = YES;
  }
  systray_ready();
}

- (void)updateTitleButtonStyle {
  if (gStatusItem.button.image != nil) {
    if ([gStatusItem.button.title length] == 0) {
      gStatusItem.button.imagePosition = NSImageOnly;
    } else {
      gStatusItem.button.imagePosition = NSImageLeft;
    }
  } else {
    gStatusItem.button.imagePosition = NSNoImage;
  }
}

- (void)setIcon:(NSImage *)image {
  gStatusItem.button.image = image;
  [self updateTitleButtonStyle];
}

- (void)setTitle:(NSString *)title {
  gStatusItem.button.title = title;
  [self updateTitleButtonStyle];
}

- (void)setTooltip:(NSString *)tooltip {
  gStatusItem.button.toolTip = tooltip;
}

- (IBAction)menuHandler:(id)sender {
  NSNumber* menuId = [sender representedObject];
  systray_menu_item_selected(menuId.intValue);
}

- (void)add_or_update_menu_item:(MenuItem *)item {
  NSMenu *theMenu = gMenu;
  NSMenuItem *parentItem;
  if ([item->parentMenuId integerValue] > 0) {
    parentItem = find_menu_item(gMenu, item->parentMenuId);
    if (parentItem.hasSubmenu) {
      theMenu = parentItem.submenu;
    } else {
      theMenu = [[NSMenu alloc] init];
      [theMenu setAutoenablesItems:NO];
      [parentItem setSubmenu:theMenu];
    }
  }

  NSMenuItem *menuItem;
  menuItem = find_menu_item(theMenu, item->menuId);
  if (menuItem == NULL) {
    menuItem = [theMenu addItemWithTitle:item->title
                                  action:@selector(menuHandler:)
                           keyEquivalent:@""];
    [menuItem setRepresentedObject:item->menuId];
  }
  [menuItem setTitle:item->title];
  [menuItem setTag:[item->menuId integerValue]];
  [menuItem setTarget:self];
  [menuItem setToolTip:item->tooltip];
  if (item->disabled == 1) {
    menuItem.enabled = FALSE;
  } else {
    menuItem.enabled = TRUE;
  }
  if (item->checked == 1) {
    menuItem.state = NSControlStateValueOn;
  } else {
    menuItem.state = NSControlStateValueOff;
  }
}

- (void) add_separator:(NSNumber*) menuId
{
  [gMenu addItem: [NSMenuItem separatorItem]];
}

- (void) hide_menu_item:(NSNumber*) menuId
{
  NSMenuItem* menuItem = find_menu_item(gMenu, menuId);
  if (menuItem != NULL) {
    [menuItem setHidden:TRUE];
  }
}

- (void) setMenuItemIcon:(NSArray*)imageAndMenuId {
  NSImage* image = [imageAndMenuId objectAtIndex:0];
  NSNumber* menuId = [imageAndMenuId objectAtIndex:1];

  NSMenuItem* menuItem;
  menuItem = find_menu_item(gMenu, menuId);
  if (menuItem == NULL) {
    return;
  }
  menuItem.image = image;
}

- (void) show_menu_item:(NSNumber*) menuId
{
  NSMenuItem* menuItem = find_menu_item(gMenu, menuId);
  if (menuItem != NULL) {
    [menuItem setHidden:FALSE];
  }
}

- (void) quit
{
  [NSApp terminate:self];
}

@end

void registerSystray(void) {
  // IMPORTANT: do NOT call [NSApp setDelegate:] here. The host (Wails) already
  // owns the NSApplicationDelegate that drives applicationShouldTerminate:,
  // OnBeforeClose, OnShutdown and the Dock context menu. Overwriting that
  // delegate caused macOS to expose only "Force Quit" instead of the normal
  // "Quit" item and skipped graceful shutdown of the proxy / kill switch /
  // system network settings.
  //
  // Allocate the controller synchronously so subsequent runInMainThread calls
  // have a valid target, and schedule the actual NSStatusItem creation on the
  // main run loop (Wails has already started [NSApp run] by the time we get
  // here, so the block will be picked up on the next iteration).
  if (gSystrayController == nil) {
    gSystrayController = [[SystrayController alloc] init];
  }

  // Block until the NSStatusItem exists on the main thread. dispatch_async here
  // raced with Go's onReady (SetIcon / AddMenuItem) and could leave gStatusItem
  // nil or deadlock the main thread via performSelectorOnMainThread:waitUntilDone:YES.
  void (^setupBlock)(void) = ^{
    [gSystrayController setupOnMainThread];

    if (gSystrayTerminateObserver == nil) {
      gSystrayTerminateObserver = [[NSNotificationCenter defaultCenter]
          addObserverForName:NSApplicationWillTerminateNotification
                      object:nil
                       queue:[NSOperationQueue mainQueue]
                  usingBlock:^(__unused NSNotification *note) {
                    systray_on_exit();
                  }];
    }
  };
  if ([NSThread isMainThread]) {
    setupBlock();
  } else {
    dispatch_sync(dispatch_get_main_queue(), setupBlock);
  }
}

int nativeLoop(void) {
  // Wails owns the NSApp main run loop on macOS. The legacy implementation
  // tried to call [NSApp run] from the systray goroutine's OS thread, which
  // is unsupported (NSApp run must run on the main thread) and conflicted
  // with the host's existing run loop. With registerSystray() dispatching
  // the NSStatusItem setup onto the main queue and an
  // NSApplicationWillTerminateNotification observer driving on-exit, the
  // systray goroutine has nothing left to do — return immediately and let
  // the goroutine exit.
  return EXIT_SUCCESS;
}

void runInMainThread(SEL method, id object) {
  if (gSystrayController == nil) {
    return;
  }
  void (^work)(void) = ^{
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
    if (object != nil) {
      [gSystrayController performSelector:method withObject:object];
    } else {
      [gSystrayController performSelector:method];
    }
#pragma clang diagnostic pop
  };
  // Never use performSelectorOnMainThread:waitUntilDone:YES from the main
  // thread — that deadlocks Cocoa and makes macOS show only "Force Quit" in
  // the Dock menu while the UI (including the status item) stops responding.
  if ([NSThread isMainThread]) {
    work();
  } else {
    dispatch_sync(dispatch_get_main_queue(), work);
  }
}

void setIcon(const char* iconBytes, int length, bool template) {
  NSData* buffer = [NSData dataWithBytes: iconBytes length:length];
  NSImage *image = [[NSImage alloc] initWithData:buffer];
  [image setSize:NSMakeSize(16, 16)];
  image.template = template;
  runInMainThread(@selector(setIcon:), (id)image);
}

void setMenuItemIcon(const char* iconBytes, int length, int menuId, bool template) {
  NSData* buffer = [NSData dataWithBytes: iconBytes length:length];
  NSImage *image = [[NSImage alloc] initWithData:buffer];
  [image setSize:NSMakeSize(16, 16)];
  image.template = template;
  NSNumber *mId = [NSNumber numberWithInt:menuId];
  runInMainThread(@selector(setMenuItemIcon:), @[image, (id)mId]);
}

void setTitle(char* ctitle) {
  NSString* title = [[NSString alloc] initWithCString:ctitle
                                             encoding:NSUTF8StringEncoding];
  free(ctitle);
  runInMainThread(@selector(setTitle:), (id)title);
}

void setTooltip(char* ctooltip) {
  NSString* tooltip = [[NSString alloc] initWithCString:ctooltip
                                               encoding:NSUTF8StringEncoding];
  free(ctooltip);
  runInMainThread(@selector(setTooltip:), (id)tooltip);
}

void add_or_update_menu_item(int menuId, int parentMenuId, char* title, char* tooltip, short disabled, short checked, short isCheckable) {
  MenuItem* item = [[MenuItem alloc] initWithId: menuId withParentMenuId: parentMenuId withTitle: title withTooltip: tooltip withDisabled: disabled withChecked: checked];
  free(title);
  free(tooltip);
  runInMainThread(@selector(add_or_update_menu_item:), (id)item);
}

void add_separator(int menuId) {
  NSNumber *mId = [NSNumber numberWithInt:menuId];
  runInMainThread(@selector(add_separator:), (id)mId);
}

void hide_menu_item(int menuId) {
  NSNumber *mId = [NSNumber numberWithInt:menuId];
  runInMainThread(@selector(hide_menu_item:), (id)mId);
}

void show_menu_item(int menuId) {
  NSNumber *mId = [NSNumber numberWithInt:menuId];
  runInMainThread(@selector(show_menu_item:), (id)mId);
}

void quit() {
  runInMainThread(@selector(quit), nil);
}
